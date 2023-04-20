package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"text/template"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bwmarrin/discordgo"
	"github.com/chzyer/readline"
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

	bot := &TorontoBot{
		prompt: prompt,
		ai:     ai,
		db:     db,
	}

	if *discordBotToken != "" {
		ds, err := discordgo.New("Bot " + *discordBotToken)
		if err != nil {
			log.Fatalf("Error creating Discord session: %v", err)
		}
		ds.AddHandler(bot.slashCommandHandler)
		ds.AddHandler(func(s *discordgo.Session, _ *discordgo.Ready) {
			err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
				Status: "online",
			})
			if err != nil {
				log.Println("Error updating bot status:", err)
			}
		})
		if err = ds.Open(); err != nil {
			log.Fatalf("Error opening Discord connection: %v", err)
		}
		defer ds.Close()
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
	}

	rl, err := readline.New(">> ")
	if err != nil {
		log.Fatal(err)
	}
	// loop to read commands and print output
	for {
		question, err := rl.Readline()
		if err != nil {
			break
		}
		sqlAnalysis, err := bot.SQLAnalysis(question)
		if err != nil {
			fmt.Println("Error analyzing SQL query:", err)
			continue
		}

		fmt.Printf(
			"%s\n\n%s\n\nSQL: %q\n",
			sqlAnalysis.Schema,
			sqlAnalysis.Applicability,
			sqlAnalysis.SQL)

		resultsTable, err := bot.ExecuteQuery(sqlAnalysis.SQL)
		if err != nil {
			fmt.Println("Error executing SQL query:", err)
			continue
		}

		fmt.Printf("\nQuery result:\n```%s```\n", resultsTable)
	}
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

			sqlAnalysis, err := b.SQLAnalysis(option.StringValue())
			if err != nil {
				errMsg := fmt.Sprintf("Error analyzing SQL query: %v", err)
				_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: &errMsg,
				})
				if err != nil {
					log.Println("Error editing initial response:", err)
				}
				return
			}

			out := fmt.Sprintf(
				"Question: *%s*\n\n%s\n\n%s\n\nExecuting query %q\n",
				option.StringValue(),
				sqlAnalysis.Schema,
				sqlAnalysis.Applicability,
				sqlAnalysis.SQL)
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

			resultsTable, err := b.ExecuteQuery(sqlAnalysis.SQL)
			if err != nil {
				errMsg := fmt.Sprintf("Error executing SQL query: %v", err)
				_, err = ds.FollowupMessageEdit(i.Interaction, followupMessage.ID, &discordgo.WebhookEdit{
					Content: &errMsg,
				})
				if err != nil {
					log.Println("Error editing initial response:", err)
				}
				return
			}

			msg := resultsTable
			if len(resultsTable) > 1900 {
				msg = resultsTable[:1900] + "..."
			}

			results := fmt.Sprintf("\nQuery result:\n```%s```\n", msg)
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

type SQLResponse struct {
	Schema        string
	Applicability string
	SQL           string
}

func (b *TorontoBot) SQLAnalysis(question string) (*SQLResponse, error) {
	var query bytes.Buffer
	data := struct {
		Command string
	}{
		Command: question,
	}
	if err := b.prompt.Execute(&query, data); err != nil {
		return nil, fmt.Errorf("executing template: %+v", err)
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
		return nil, fmt.Errorf("CreateChatCompletion: %v", err)
	}
	log.Printf("Got reply: %s\n", aiResp.Choices[0].Message.Content)

	var resp SQLResponse
	if err := json.Unmarshal([]byte(aiResp.Choices[0].Message.Content), &resp); err != nil {
		return nil, fmt.Errorf("unmarshalling response %q: %v", aiResp.Choices[0].Message.Content, err)
	}
	return &resp, nil
}

// ExecuteQuery takes an SQL query and executes it against the database, then returns a nicely
// formatted ascii table with the results.
func (b *TorontoBot) ExecuteQuery(query string) (string, error) {
	p := message.NewPrinter(language.English)

	rows, err := b.db.Query(query)
	if err != nil {
		return "", fmt.Errorf("query: %v", err)
	}
	defer rows.Close()

	log.Println("Got SQL response")

	columnNames, _ := rows.Columns()
	columnCount := len(columnNames)
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return "", fmt.Errorf("getting column types: %v", err)
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
			return "", fmt.Errorf("error scanning row: %v", err)
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

	return tw.Render(), nil
}
