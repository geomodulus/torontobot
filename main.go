package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"text/template"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/sashabaranov/go-openai"
)

const (
	// Model is the openai model to query. GPT-4 is expensive, so we use GPT-3.5.
	Model = openai.GPT3Dot5Turbo
	// RespTemp is the response temperature we want from the model. Default temp is 1.0 and higher
	// is more "creative".
	RespTemp = 0.5

	// Torontoverse Discord server ID
	GuildID = "1023614976772030605"
)

type Response struct {
	Schema        string
	Applicability string
	SQL           string
}

func main() {
	dbFile := flag.String("db-file", "./db/toronto.db", "Database file for tabular city data")
	discordBotToken := flag.String("discord-bot-token", "", "Token for accessing Discord API")
	openaiToken := flag.String("openai-token", "", "Token for accessing OpenAI API")

	flag.Parse()

	// Read and parse the template file
	prompt, err := template.ParseFiles("prompt.txt")
	if err != nil {
		fmt.Println("Error parsing template:", err)
		return
	}
	ai := openai.NewClient(*openaiToken)

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ds, err := discordgo.New("Bot " + *discordBotToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	bot := &TorontoBot{
		prompt: prompt,
		ai:     ai,
		db:     db,
	}

	ds.AddHandler(bot.slashCommandHandler)

	if err = ds.Open(); err != nil {
		log.Fatalf("Error opening Discord connection: %v", err)
	}

	cmd, err := ds.ApplicationCommandCreate(ds.State.User.ID, GuildID, &discordgo.ApplicationCommand{
		Name:        "torontobot",
		Description: "Responds to questions about city of Toronto Open Data.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "question",
				Description: "Question about Toronto open data",
				Required:    true,
			},
		},
	})
	if err != nil {
		log.Fatalf("Cannot create Discord command: %v", err)
	}
	defer func() {
		if err := ds.ApplicationCommandDelete(ds.State.User.ID, GuildID, cmd.ID); err != nil {
			log.Fatalf("Cannot delete Discord command: %v", err)
		}
	}()

	fmt.Println("TorontoBot is now live on Discord. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	ds.Close()
}

type TorontoBot struct {
	prompt *template.Template
	ai     *openai.Client
	db     *sql.DB
}

func (b *TorontoBot) slashCommandHandler(ds *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	for _, option := range i.ApplicationCommandData().Options {
		if option.Name == "question" {
			if err := ds.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			}); err != nil {
				fmt.Println("Error sending deferred response:", err)
				return
			}

			p := message.NewPrinter(language.English)

			var query bytes.Buffer
			data := struct {
				Command string
			}{
				Command: option.StringValue(),
			}
			if err := b.prompt.Execute(&query, data); err != nil {
				log.Fatal("Error executing template:", err)
			}
			log.Printf("sending request to openai: %q\n", query.String())
			aiResp, err := b.ai.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
				Model: openai.GPT3Dot5Turbo,
				Messages: []openai.ChatCompletionMessage{{
					Role:    openai.ChatMessageRoleUser,
					Content: query.String(),
				}},
				Temperature: RespTemp,
			})
			if err != nil {
				log.Fatalf("CreateChatCompletion: %v", err)
			}
			log.Printf("Got reply: %s\n", aiResp.Choices[0].Message.Content)

			var resp Response
			if err := json.Unmarshal([]byte(aiResp.Choices[0].Message.Content), &resp); err != nil {
				log.Fatalf("Error unmarshalling response %q: %v", aiResp.Choices[0].Message.Content, err)
			}

			out := fmt.Sprintf("Question: *%s*\n\n%s\n\n%s\n\nExecuted query %q\n", option.StringValue(), resp.Schema, resp.Applicability, resp.SQL)
			// Edit the original deferred response with the actual content
			_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &out,
			})
			if err != nil {
				log.Println("Error editing initial response:", err)
			}

			_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &out,
			})
			if err != nil {
				log.Println("Error editing initial response:", err)
			}
			followupMessage, err := ds.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: "Querying database...",
			})
			if err != nil {
				fmt.Println("Error sending follow-up message:", err)
				return
			}

			rows, err := b.db.Query(resp.SQL)
			if err != nil {
				log.Fatalf("Query: %v", err)
			}
			defer rows.Close()

			log.Println("Got SQL response")

			columnNames, _ := rows.Columns()
			columnCount := len(columnNames)
			columnTypes, err := rows.ColumnTypes()
			if err != nil {
				log.Printf("Error getting column types: %v\n", err)
				continue
			}

			// Create a table writer and set column headers
			tw := table.NewWriter()
			header := make(table.Row, columnCount)
			for i, columnName := range columnNames {
				header[i] = fmt.Sprintf("%s (%s)", columnName, columnTypes[i].DatabaseTypeName())
			}
			tw.AppendHeader(header)

			for rows.Next() {
				columns := make([]interface{}, columnCount)
				columnPointers := make([]interface{}, columnCount)

				for i := range columns {
					columnPointers[i] = &columns[i]
				}

				if err := rows.Scan(columnPointers...); err != nil {
					log.Printf("Error scanning row: %v\n", err)
					continue
				}

				row := make(table.Row, columnCount)
				for i, column := range columns {
					if columnTypes[i].DatabaseTypeName() == "REAL" || columnTypes[i].DatabaseTypeName() == "" {
						row[i] = p.Sprintf("$%.2f", column)
						continue
					}
					row[i] = p.Sprintf("%+v", column)
				}
				tw.AppendRow(row)
			}
			rows.Close()

			results := fmt.Sprintf("\nQuery result:\n```%s```\n", tw.Render())
			// Edit the original deferred response with the actual content
			_, err = ds.FollowupMessageEdit(i.Interaction, followupMessage.ID, &discordgo.WebhookEdit{
				Content: &results,
			})
			if err != nil {
				log.Println("Error editing follow-up message:", err)
			}

		}
	}

}
