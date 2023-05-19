package main

import (
	"bytes"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bwmarrin/discordgo"
	"github.com/chzyer/readline"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc"

	"github.com/geomodulus/citygraph"
	"github.com/geomodulus/torontobot/bot"
	"github.com/geomodulus/torontobot/storage"
	"github.com/geomodulus/torontobot/viz"
)

const (
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

	ai := openai.NewClient(*openaiToken)

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	tb, err := bot.New(db, ai, store, *hostname)
	if err != nil {
		log.Fatalf("Error creating bot: %s", err)
	}

	if *discordBotToken != "" {
		discordBotServer, err := OpenDiscordBotServer(*discordBotToken, tb)
		if err != nil {
			log.Fatalf("Error opening Discord bot server: %s", err)
		}
		defer discordBotServer.Close()

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
		sqlAnalysis, err := tb.SQLAnalysis(ctx, question)
		if err != nil {
			fmt.Println("Error analyzing SQL query:", err)
			continue
		}

		if sqlAnalysis.MissingData != "" {
			fmt.Printf(
				"I can't answer that: %s\n\n%s\n",
				sqlAnalysis.MissingData,
				sqlAnalysis.Applicability)
			continue
		}

		fmt.Printf(
			"%s\n\n%s\n\nSQL: %q\n",
			sqlAnalysis.Schema,
			sqlAnalysis.Applicability,
			sqlAnalysis.SQL)

		resultsTable, err := tb.LoadResults(sqlAnalysis.SQL)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Println("No results found.")
			} else {
				fmt.Println("Error executing SQL query:", err)
			}
			continue
		}

		fmt.Printf("\nQuery result:\n```%s```\n", resultsTable)

		chartSelected, err := tb.SelectChart(ctx, question, resultsTable)
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
				id := citygraph.NewID().String()

				featureImageURL, err := generateAndUploadFeatureImage(
					ctx,
					id,
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency,
				)
				if err != nil {
					fmt.Println("Error generating feature image:", err)
					continue
				}
				modPath, err := tb.SaveToGraph(
					ctx,
					id,
					question,
					renderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
					js,
					featureImageURL,
					"Local User")
				if err != nil {
					fmt.Println("Error saving chart to graph:", err)
					continue
				}
				fmt.Printf("Published chart at %s\n", tb.Hostname+modPath)
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
				modPath, err := tb.SaveToGraph(
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
				fmt.Printf("Published chart at %s\n", tb.Hostname+modPath)
			}
		case "pie chart":
			//case "scatter plot":

		default:
			fmt.Printf("Ah you need a %s, but I can't make those yet. Soon 😈\n", chartSelected.Chart)
		}
	}
}

type DiscordBotServer struct {
	session *discordgo.Session
	bot     *bot.TorontoBot
	cmd     *discordgo.ApplicationCommand
}

func OpenDiscordBotServer(token string, tb *bot.TorontoBot) (*DiscordBotServer, error) {
	ds, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %v", err)
	}
	s := &DiscordBotServer{
		session: ds,
		bot:     tb,
	}
	s.session.AddHandler(s.slashCommandHandler)
	if err = s.session.Open(); err != nil {
		return nil, fmt.Errorf("error opening Discord connection: %v", err)
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
		return nil, fmt.Errorf("error creating Discord command: %v", err)
	}
	s.cmd = cmd
	return s, nil
}

func (s *DiscordBotServer) Close() error {
	if err := s.session.ApplicationCommandDelete(s.session.State.User.ID, GuildID, s.cmd.ID); err != nil {
		return fmt.Errorf("error deleting Discord command: %v", err)
	}
	return s.session.Close()
}

func (s *DiscordBotServer) slashCommandHandler(ds *discordgo.Session, i *discordgo.InteractionCreate) {
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

			sqlAnalysis, err := s.bot.SQLAnalysis(ctx, question)
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

			var out string
			if sqlAnalysis.MissingData != "" {
				out = fmt.Sprintf(
					"Question: *%s*\n\nI can't answer that: %s\n\n%s\n",
					question,
					sqlAnalysis.Applicability,
					sqlAnalysis.MissingData)
			} else {
				out = fmt.Sprintf(
					"Question: *%s*\n\n%s\n\nExecuting query `%s`\n",
					question,
					sqlAnalysis.Applicability,
					sqlAnalysis.SQL)
			}
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

			resultsTable, err := s.bot.LoadResults(sqlAnalysis.SQL)
			if err != nil {
				var errMsg string
				if err == sql.ErrNoRows {
					errMsg = "No results found for that query. Try again?"
				} else {
					errMsg = fmt.Sprintf("Error reading data: %v", err)
				}
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

			chartSelected, err := s.bot.SelectChart(ctx, question, resultsTable)
			if err != nil {
				fmt.Println("Error selecting chart:", err)
				continue
			}
			switch strings.ToLower(chartSelected.Chart) {
			case "bar chart":
				chartHTML, err := viz.GenerateBarChartHTML(
					chartSelected.Title,
					chartSelected.Data,
					chartSelected.ValueIsCurrency,
					false, // not dark mode
					viz.WithFixedWidth(675),
					viz.WithFixedHeight(750),
				)
				if err != nil {
					fmt.Println("Error generating HTML:", err)
					continue
				}
				pngBytes, err := viz.ScreenshotHTML(
					ctx,
					chartHTML,
					viz.WithWidth(675),
					viz.WithHeight(750),
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
				if s.bot.HasGraphStore() {
					// Send the chart!
					followupMessage, err = ds.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
						Content: fmt.Sprintf("Publishing to %s...", s.bot.Hostname),
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
					id := citygraph.NewID().String()
					featureImageURL, err := generateAndUploadFeatureImage(
						ctx,
						id,
						chartSelected.Title,
						chartSelected.Data,
						chartSelected.ValueIsCurrency,
					)
					if err != nil {
						fmt.Println("Error generating feature image:", err)
						continue
					}
					modPath, err := s.bot.SaveToGraph(
						ctx,
						id,
						question,
						renderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
						js,
						featureImageURL,
						i.Member.User.Username)
					if err != nil {
						fmt.Println("Error saving to graph:", err)
						continue
					}

					out := fmt.Sprintf("Published chart at %s\n", s.bot.Hostname+modPath)
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

func generateAndUploadFeatureImage(ctx context.Context, id, title string, data []*viz.DataEntry, isCurrency bool) (string, error) {
	chartHTML, err := viz.GenerateBarChartHTML(
		title, data, isCurrency, true, //  yes to dark mode
		viz.WithFixedWidth(800),
		viz.WithFixedHeight(750),
	)
	if err != nil {
		return "", fmt.Errorf("generating bar chart: %v", err)
	}
	pngBytes, err := viz.ScreenshotHTML(ctx, chartHTML, viz.WithWidth(800), viz.WithHeight(450), viz.WithScale(2))
	if err != nil {
		return "", fmt.Errorf("generating PNG: %v", err)
	}
	featureImageObject := id + ".png"
	if err := storage.UploadToGCS(ctx, featureImageObject, bytes.NewReader(pngBytes)); err != nil {
		return "", fmt.Errorf("saving chart to GCS: %v", err)
	}
	return "https://dev.geomodul.us/dev-charts/" + id + ".png", nil
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
