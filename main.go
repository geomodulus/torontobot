package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bwmarrin/discordgo"
	"github.com/chzyer/readline"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc"

	"github.com/geomodulus/citygraph"
	"github.com/geomodulus/torontobot/db/reader"
	"github.com/geomodulus/torontobot/storage"
	"github.com/geomodulus/torontobot/viz"
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

	var store *citygraph.Store
	if *citygraphAddr != "" {
		graphConn, err := grpc.Dial(*citygraphAddr, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}
		defer graphConn.Close()
		store = &citygraph.Store{GraphClient: citygraph.NewClient(graphConn)}
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
			if store != nil {
				js, err := viz.GenerateBarChartJS(
					"#torontobot-chart",
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency,
					viz.WithBreakpointWidth())
				if err != nil {
					fmt.Println("Error generating JS:", err)
					continue
				}
				pngBytes, err := bot.GenerateBarChartPNG(
					ctx,
					1200, 900,
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency,
					viz.WithFixedWidth(1200),
					viz.WithFixedHeight(1400),
				)
				if err != nil {
					fmt.Println("Error generating PNG:", err)
					continue
				}
				id := citygraph.NewID().String()
				featureImageObject := id + ".png"
				if err := storage.UploadToGCS(ctx, featureImageObject, bytes.NewReader(pngBytes)); err != nil {
					fmt.Println("Error saving chart to GCS:", err)
					continue
				}
				modPath, err := bot.SaveToGraph(
					ctx,
					id,
					question,
					renderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
					js,
					"https://dev.geomodul.us/dev-charts/"+featureImageObject,
					"Local User")
				if err != nil {
					fmt.Println("Error saving chart to graph:", err)
					continue
				}
				fmt.Printf("Published chart at %s\n", bot.hostname+modPath)
			}

		case "line chart":
			if store != nil {
				js, err := viz.GenerateLineChartJS(
					"#torontobot-chart",
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency,
					viz.WithBreakpointWidth())
				if err != nil {
					fmt.Println("Error generating JS:", err)
					continue
				}
				id := citygraph.NewID().String()
				modPath, err := bot.SaveToGraph(
					ctx,
					id,
					question,
					renderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
					js,
					"",
					"Local User")
				if err != nil {
					fmt.Println("Error saving chart to graph:", err)
					continue
				}
				fmt.Printf("Published chart at %s\n", bot.hostname+modPath)
			}
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
	graphStore        *citygraph.Store
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
			question := option.StringValue()

			sqlAnalysis, err := b.SQLAnalysis(ctx, question)
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
				"Question: *%s*\n\n%s\n\nExecuting query `%s`\n",
				question,
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

			chartSelected, err := b.SelectChart(ctx, question, resultsTable)
			if err != nil {
				fmt.Println("Error selecting chart:", err)
				continue
			}
			switch strings.ToLower(chartSelected.Chart) {
			case "bar chart":
				pngBytes, err := b.GenerateBarChartPNG(
					ctx,
					675, 750,
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency,
					viz.WithFixedWidth(675),
					viz.WithFixedHeight(750),
				)
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

					js, err := viz.GenerateBarChartJS(
						"#torontobot-chart",
						chartSelected.Title,
						chartSelected.Data,
						chartSelected.ValueIsCurrency,
						viz.WithBreakpointWidth())
					if err != nil {
						fmt.Println("Error generating JS:", err)
						continue
					}
					shareImageBytes, err := b.GenerateBarChartPNG(
						ctx,
						1200, 900,
						chartSelected.Title,
						chartSelected.Data,
						chartSelected.ValueIsCurrency,
						viz.WithFixedWidth(1200),
						viz.WithFixedHeight(1400),
					)
					if err != nil {
						fmt.Println("Error generating PNG:", err)
						continue
					}
					id := citygraph.NewID().String()
					featureImageObject := id + ".png"
					if err := storage.UploadToGCS(ctx, featureImageObject, bytes.NewReader(shareImageBytes)); err != nil {
						fmt.Println("Error saving chart to GCS:", err)
						continue
					}
					modPath, err := b.SaveToGraph(
						ctx,
						id,
						question,
						renderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
						js,
						"https://dev.geomodul.us/dev-charts/"+featureImageObject,
						i.Member.User.Username)
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
		Date    string
		Command string
	}{
		Date:    time.Now().Format("January 2, 2006"),
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

func (b *TorontoBot) GenerateBarChartPNG(ctx context.Context, width, height float64, title string, data []*viz.DataEntry, isCurrency bool, chartOptions ...viz.ChartOption) ([]byte, error) {
	chartHTML, err := viz.GenerateBarChartHTML(title, data, isCurrency, chartOptions...)
	if err != nil {
		return []byte{}, fmt.Errorf("generating bar chart: %v", err)
	}
	//	filename := "../mainapp/static/dev-chart.html"
	//	if err := ioutil.WriteFile(filename, []byte(chartHTML), 0644); err != nil {
	//		return []byte{}, fmt.Errorf("writing chart.html: %v", err)
	//	}
	//	fmt.Println("Wrote", filename)
	return viz.SVGToPNG(ctx, width, height, chartHTML)
}

func renderBody(question, schemaThoughts, analysis, sqlQuery string) string {
	return `
				<figure>
				  <div id="torontobot-chart"></div>
				  <figcaption>Data from: Operating Budget Program Summary by Expenditure Category, 2014 - 2023
				    Source: <a href="https://open.toronto.ca/dataset/budget-operating-budget-program-summary-by-expenditure-category/" target="_blank">
				    City of Toronto Open Data</a>
				  </figcaption>
				</figure>
				<p>This chart was generated using an experimental AI-powered open data query tool called 
				<a href="https://github.com/geomodulus/torontobot" target="_blank">TorontoBot</a>.</p>
				<p>Want to generate your own or help contribute to the project?
				<a href="https://discord.gg/sQzxHBq8Q2" target="_blank">Join our Discord</a>.</p>
				<ins class="geomodcau"></ins>
				<h3>How does it work?</h3>
				<p>First, the bot uses GPT-3 to analyze the question and generate a SQL query.</p>
				<p>Then, the bot uses a custom SQL query engine to query a database we've filled
				with data from the City of Toronto Open Data portal.</p>
				<p>Finally, the bot uses a custom charting engine to generate a chart from the results.</p>
				<h3>What does the bot think?</h3>
				<h5 class="font-bold">Question</h5>
				<p>` + question + `</p>
				<h5 class="font-bold">AI thought process</h5>
				<p><em>` + schemaThoughts + `</em></p>
				<p><em>` + analysis + `</em></p>
				<h5 class="font-bold">SQL Query</h5>
				<p class="p-2 bg-map-800 text-map-200"><code>` + sqlQuery + `</code></p>`
}

func (b *TorontoBot) SaveToGraph(ctx context.Context, id, title, body, js, featureImage, user string) (string, error) {
	camera := map[string]interface{}{
		"": map[string]interface{}{
			"center":  map[string]float64{"lng": -79.384, "lat": 43.645},
			"zoom":    13.8,
			"pitch":   0,
			"bearing": -30,
		}}
	mod := &citygraph.Module{
		ID:           id,
		Name:         title,
		Headline:     fmt.Sprintf("<h1>City Budget: %s</h1>", title),
		Categories:   []string{"Open Data"},
		Creators:     []string{user},
		Camera:       camera,
		FeatureImage: featureImage,
		Description:  "User-generated open data visualization",
		PubDate:      time.Now().Format("2006-01-02"),
		CodeCredit:   "TorontoBot, an open data bot",
	}
	if err := b.graphStore.WriteModule(ctx, mod); err != nil {
		return "", fmt.Errorf("writing module: %v", err)
	}

	q, err := mod.VertexQuery()
	if err != nil {
		return "", fmt.Errorf("generating vertex query: %v", err)
	}
	if err := b.graphStore.WriteBodyText(ctx, q, body); err != nil {
		return "", fmt.Errorf("writing body text: %v", err)
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
