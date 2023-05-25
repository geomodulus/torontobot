package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chzyer/readline"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc"

	"github.com/geomodulus/citygraph"
	"github.com/geomodulus/torontobot/bot"
	uq "github.com/geomodulus/torontobot/db"
	"github.com/geomodulus/torontobot/discord"
	"github.com/geomodulus/torontobot/viz"
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
		discordBotServer, err := discord.OpenBotServer(db, *discordBotToken, tb)
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
		// Store query for subsequent charting and export,
		_, err = uq.StoreUserQuery(
			db,
			&uq.UserQuery{
				"",
				"",
				"",
				question,
				sqlAnalysis,
				resultsTable,
				time.Time{},
			})
		if err != nil {
			log.Println("Error storing query:", err)
			return
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

				featureImageURL, err := viz.GenerateAndUploadFeatureImage(
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
					viz.RenderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
					js,
					featureImageURL,
					"Local User")
				if err != nil {
					fmt.Println("Error saving chart to graph:", err)
					continue
				}
				fmt.Printf("Published chart at %s\n", tb.Hostname+modPath)
			}

			//		case "line chart":
			//			if store != nil {
			//				js, err := viz.GenerateLineChartJS(
			//					"#torontobot-chart",
			//					chartSelected.Title,
			//					chartSelected.Data,
			//					chartSelected.ValueIsCurrency,
			//					viz.WithBreakpointWidth())
			//				if err != nil {
			//					fmt.Println("Error generating JS:", err)
			//					continue
			//				}
			//				id := citygraph.NewID().String()
			//				modPath, err := tb.SaveToGraph(
			//					ctx,
			//					id,
			//					question,
			//					viz.RenderBody(question, sqlAnalysis.Schema, sqlAnalysis.Applicability, sqlAnalysis.SQL),
			//					js,
			//					"",
			//					"Local User")
			//				if err != nil {
			//					fmt.Println("Error saving chart to graph:", err)
			//					continue
			//				}
			//				fmt.Printf("Published chart at %s\n", tb.Hostname+modPath)
			//			}
			//			//case "pie chart":
			//			//case "scatter plot":
			//
		default:
			fmt.Printf("Ah you need a %s, but I can't make those yet. Soon 😈\n", chartSelected.Chart)
		}
	}
}
