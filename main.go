package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"text/template"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bwmarrin/discordgo"
	"github.com/chzyer/readline"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc"

	"github.com/geomodulus/corpus/graph"
	"github.com/geomodulus/torontobot/db/reader"
	"github.com/geomodulus/torontobot/viz"
	"github.com/geomodulus/witness/citygraph"
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
	// For dev use: "127.0.0.1:27615"
	citygraphAddr := flag.String("citygraph-addr", "", "address string for citygraph indradb GRPC server")
	dbFile := flag.String("db-file", "./db/toronto.db", "Database file for tabular city data")
	discordBotToken := flag.String("discord-bot-token", "", "Token for accessing Discord API")
	openaiToken := flag.String("openai-token", "", "Token for accessing OpenAI API")
	hostname := flag.String("host", "https://torontoverse.com", "host and scheme for torontoverse server")

	flag.Parse()

	ctx := context.Background()

	var store *graph.Store
	if *citygraphAddr != "" {
		graphConn, err := grpc.Dial(*citygraphAddr, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}
		defer graphConn.Close()
		store = &graph.Store{GraphClient: citygraph.NewClient(graphConn)}
	}

	sqlGenPrompt, err := template.ParseFiles("./prompts/sql_gen.txt")
	if err != nil {
		fmt.Println("Error parsing template:", err)
		return
	}
	chartSelectPrompt, err := template.ParseFiles("./prompts/chart_select.txt")
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
		hostname:          *hostname,
		sqlGenPrompt:      sqlGenPrompt,
		chartSelectPrompt: chartSelectPrompt,
		graphStore:        store,
		ai:                ai,
		db:                db,
	}

	if *discordBotToken != "" {
		ds, err := discordgo.New("Bot " + *discordBotToken)
		if err != nil {
			log.Fatalf("Error creating Discord session: %v", err)
		}
		ds.AddHandler(bot.slashCommandHandler)
		//		ds.AddHandler(func(s *discordgo.Session, _ *discordgo.Ready) {
		//			err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
		//				Status: "online",
		//			})
		//			if err != nil {
		//				log.Println("Error updating bot status:", err)
		//			}
		//		})
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
		if strings.TrimSpace(question) == "" {
			continue
		}
		sqlAnalysis, err := bot.SQLAnalysis(ctx, question)
		if err != nil {
			fmt.Println("Error analyzing SQL query:", err)
			continue
		}

		fmt.Printf(
			"%s\n\n%s\n\nSQL: %q\n",
			sqlAnalysis.Schema,
			sqlAnalysis.Applicability,
			sqlAnalysis.SQL)

		resultsTable, err := reader.ReadDataTable(bot.db, sqlAnalysis.SQL)
		if err != nil {
			fmt.Println("Error executing SQL query:", err)
			continue
		}

		fmt.Printf("\nQuery result:\n```%s```\n", resultsTable)

		chartSelected, err := bot.SelectChart(ctx, question, resultsTable)
		if err != nil {
			fmt.Println("Error selecting chart:", err)
			continue
		}
		switch strings.ToLower(chartSelected.Chart) {
		case "bar chart":
			//	fmt.Println("Generating bar chart...")
			//	pngBytes, err := bot.GenerateBarChartPNG(
			//		ctx,
			//		chartSelected.Title,
			//		chartSelected.Data,
			//		chartSelected.ValueIsCurrency)
			//	if err != nil {
			//		fmt.Println("Error generating PNG:", err)
			//		continue
			//	}
			//		if err = ioutil.WriteFile("../mainapp/static/dev-chart.png", pngBytes, 0644); err != nil {
			//			fmt.Println("Error writing file:", err)
			//			continue
			//		}

			if store != nil {
				modPath, err := bot.SaveToGraph(ctx, chartSelected, "Local User")
				if err != nil {
					fmt.Println("Error saving chart to graph:", err)
					continue
				}
				fmt.Printf("Published chart at %s\n", bot.hostname+modPath)
			}

			//case "line chart":
			//case "pie chart":
			//case "scatter plot":

		default:
			fmt.Printf("Ah you need a %s, but I can't make those yet. Soon 😈\n", chartSelected.Chart)
		}
	}
}

type TorontoBot struct {
	hostname          string
	sqlGenPrompt      *template.Template
	chartSelectPrompt *template.Template
	graphStore        *graph.Store
	ai                *openai.Client
	db                *sql.DB
}

func (b *TorontoBot) slashCommandHandler(ds *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()
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

			sqlAnalysis, err := b.SQLAnalysis(ctx, option.StringValue())
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

			resultsTable, err := reader.ReadDataTable(b.db, sqlAnalysis.SQL)
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

			// Send the chart!
			followupMessage, err = ds.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: "Visualizing data...",
			})
			if err != nil {
				fmt.Println("Error sending follow-up message:", err)
				return
			}

			chartSelected, err := b.SelectChart(ctx, option.StringValue(), resultsTable)
			if err != nil {
				fmt.Println("Error selecting chart:", err)
				continue
			}
			switch strings.ToLower(chartSelected.Chart) {
			case "bar chart":
				pngBytes, err := b.GenerateBarChartPNG(
					ctx,
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency)
				if err != nil {
					fmt.Println("Error generating PNG:", err)
					continue
				}
				dsFile := &discordgo.File{
					Name:   "chart.png",
					Reader: bytes.NewReader(pngBytes),
				}
				out := "Here's my attempt at a chart!"
				_, err = ds.FollowupMessageEdit(i.Interaction, followupMessage.ID, &discordgo.WebhookEdit{
					Content: &out,
					Files:   []*discordgo.File{dsFile},
				})
				if err != nil {
					fmt.Println("Error editing follow-up message:", err)
					continue
				}
				if b.graphStore != nil {
					// Send the chart!
					followupMessage, err = ds.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
						Content: fmt.Sprintf("Publishing to %s...", b.hostname),
					})
					if err != nil {
						fmt.Println("Error sending follow-up message:", err)
						return
					}

					modPath, err := b.SaveToGraph(ctx, chartSelected, i.Member.User.Username)
					if err != nil {
						fmt.Println("Error saving to graph:", err)
						continue
					}

					out := fmt.Sprintf("Published chart at %s\n", b.hostname+modPath)
					_, err = ds.FollowupMessageEdit(i.Interaction, followupMessage.ID, &discordgo.WebhookEdit{
						Content: &out,
					})
					if err != nil {
						fmt.Println("Error editing follow-up message:", err)
						continue
					}
				}

			default:
				out := fmt.Sprintf("Ah you need a %s, but I can't make those yet. Soon 😈", chartSelected.Chart)
				_, err = ds.FollowupMessageEdit(i.Interaction, followupMessage.ID, &discordgo.WebhookEdit{
					Content: &out,
				})
				if err != nil {
					log.Println("Error editing follow-up message:", err)
				}

				//case "line chart":
				//case "pie chart":
				//case "scatter plot":
			}
		}
	}

}

type SQLResponse struct {
	Schema        string
	Applicability string
	SQL           string
}

func (b *TorontoBot) SQLAnalysis(ctx context.Context, question string) (*SQLResponse, error) {
	var query bytes.Buffer
	data := struct {
		Command string
	}{
		Command: question,
	}
	if err := b.sqlGenPrompt.Execute(&query, data); err != nil {
		return nil, fmt.Errorf("executing template: %+v", err)
	}
	log.Printf("sending request to openai: %q\n", query.String())
	aiResp, err := b.ai.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: Model,
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

type ChartSelectResponse struct {
	Chart           string
	Title           string
	Data            []*viz.DataEntry
	ValueIsCurrency bool
}

type ChartType int

const (
	ChartTypeUnknown ChartType = iota
	ChartTypeBar
	ChartTypeLine
	ChartTypePie
	ChartTypeScatter
)

func (b *TorontoBot) SelectChart(ctx context.Context, question, dataTable string) (*ChartSelectResponse, error) {
	var query bytes.Buffer
	data := struct {
		Title string
		Data  string
	}{
		Title: question,
		Data:  dataTable,
	}
	if err := b.chartSelectPrompt.Execute(&query, data); err != nil {
		return nil, fmt.Errorf("executing template: %+v", err)
	}
	log.Printf("sending request to openai: %q\n", query.String())
	aiResp, err := b.ai.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: Model,
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

	var resp ChartSelectResponse
	if err := json.Unmarshal([]byte(aiResp.Choices[0].Message.Content), &resp); err != nil {
		return nil, fmt.Errorf("unmarshalling response %q: %v", aiResp.Choices[0].Message.Content, err)
	}

	fmt.Printf("%+v\n", resp)

	return &resp, nil
}

func (b *TorontoBot) GenerateBarChartPNG(ctx context.Context, title string, data []*viz.DataEntry, isCurrency bool) ([]byte, error) {
	chartHTML, err := viz.GenerateBarChartHTML(title, data, isCurrency)
	if err != nil {
		return []byte{}, fmt.Errorf("generating bar chart: %v", err)
	}
	//	filename := "../mainapp/static/dev-chart.html"
	//	if err := ioutil.WriteFile(filename, []byte(chartHTML), 0644); err != nil {
	//		return []byte{}, fmt.Errorf("writing chart.html: %v", err)
	//	}
	//	fmt.Println("Wrote", filename)
	return viz.SVGToPNG(ctx, chartHTML)
}

func (b *TorontoBot) SaveToGraph(ctx context.Context, chartSelected *ChartSelectResponse, user string) (string, error) {
	camera := map[string]interface{}{
		"": map[string]interface{}{
			"center":  map[string]float64{"lng": -79.384, "lat": 43.645},
			"zoom":    13.8,
			"pitch":   0,
			"bearing": -30,
		}}
	mod := &graph.Module{
		ID:          citygraph.NewID().String(),
		Name:        chartSelected.Title,
		Headline:    fmt.Sprintf("<h1>Toronto Budget 2023: %s</h1>", chartSelected.Title),
		Categories:  []string{"Open Data"},
		Creators:    []string{user},
		Camera:      camera,
		Description: "User-generated open data visualization",
		PubDate:     time.Now().Format("2006-01-02"),
		CodeCredit:  "TorontoBot, an open data bot",
	}
	if err := b.graphStore.WriteModule(ctx, mod); err != nil {
		return "", fmt.Errorf("writing module: %v", err)
	}

	q, err := mod.VertexQuery()
	if err != nil {
		return "", fmt.Errorf("generating vertex query: %v", err)
	}
	body := `
				<figure>
				  <div id="torontobot-chart"></div>
				  <figcaption>Data from: Operating Budget Program Summary by Expenditure Category, 2023
				    Source: <a href="https://open.toronto.ca/dataset/budget-operating-budget-program-summary-by-expenditure-category/" target="_blank">
				    City of Toronto Open Data</a>
				  </figcaption>
				</figure>
				<p>This chart was generated using an experimental AI-powered open data query tool called 
				<a href="https://github.com/geomodulus/torontobot" target="_blank">TorontoBot</a>.</p>
				<p>Want to generate your own or help contribute to the project?
				<a href="https://discord.gg/sQzxHBq8Q2" target="_blank">Join our Discord</a>.</p>
				<ins class="geomodcau"></ins>`
	if err := b.graphStore.WriteBodyText(ctx, q, body); err != nil {
		return "", fmt.Errorf("writing body text: %v", err)
	}

	js, err := viz.GenerateBarChartJS(
		"#torontobot-chart",
		chartSelected.Title,
		chartSelected.Data,
		chartSelected.ValueIsCurrency,
		viz.WithBreakpointWidth())
	if err != nil {
		return "", fmt.Errorf("generating JS: %v", err)
	}
	js += "\n\nmodule.initAdUnits();"
	if err := b.graphStore.WriteJS(ctx, q, js); err != nil {
		return "", fmt.Errorf("writing JS: %v", err)
	}

	slugID, err := mod.SlugID()
	if err != nil {
		return "", fmt.Errorf("generating slug ID: %v", err)
	}
	return fmt.Sprintf("/mod/%s/%s", slugID, mod.SlugTitle()), nil
}
