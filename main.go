package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	_ "github.com/mattn/go-sqlite3"

	"github.com/bwmarrin/discordgo"
	"github.com/chzyer/readline"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/sashabaranov/go-openai"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
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

	ctx := context.Background()

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
		sqlGenPrompt:      sqlGenPrompt,
		chartSelectPrompt: chartSelectPrompt,
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

		resultsTable, err := bot.ExecuteQuery(sqlAnalysis.SQL)
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
			pngBytes, err := bot.GenerateBarChartPNG(
				ctx,
				chartSelected.Title,
				chartSelected.Data,
				chartSelected.ValueIsCurrency)
			if err != nil {
				fmt.Println("Error generating PNG:", err)
				continue
			}
			if err = ioutil.WriteFile("../mainapp/static/dev-chart.png", pngBytes, 0644); err != nil {
				fmt.Println("Error writing file:", err)
				continue
			}

			fmt.Println("Wrote file: https://waterloo:8100/dev-chart.png")

		default:
			fmt.Printf("Ah you need a %s, but I can't make those yet. Soon 😈", chartSelected.Chart)

			//case "line chart":
			//case "pie chart":
			//case "scatter plot":
		}

	}
}

type TorontoBot struct {
	sqlGenPrompt      *template.Template
	chartSelectPrompt *template.Template
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

			// Send the graph!
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

type DataEntry struct {
	Name  string
	Value float64
}

type ChartSelectResponse struct {
	Chart           string
	Title           string
	Data            []*DataEntry
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

const htmlContent = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>SVG Export</title>

	<link href="https://torontoverse.com/css/style.css?v=7" rel="stylesheet" />

    <!-- fonts -->
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link
      href="https://fonts.googleapis.com/css2?family=JetBrains+Mono&display=swap"
      rel="stylesheet"
    />
	<script src="https://torontoverse.com/js/lib/d3/d3.min.js"></script>
</head>
<body class="bg-map-50 font-mono">
    <script type="text/javascript">
    // Add your D3.js visualization code here
	  REPLACE_ME_WITH_CHART_JS
    </script>
</body>
</html>
`

func (b *TorontoBot) GenerateBarChartPNG(ctx context.Context, title string, data []*DataEntry, isCurrency bool) ([]byte, error) {
	input := struct {
		Title      string
		Data       []*DataEntry
		IsCurrency bool
	}{
		Title:      title,
		Data:       data,
		IsCurrency: isCurrency,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return []byte{}, fmt.Errorf("marshalling data: %v", err)
	}
	jsIntro := fmt.Sprintf("const input = %s;\n", string(inputJSON))
	jsFile, err := os.ReadFile("viz/bar_chart.js")
	if err != nil {
		return []byte{}, fmt.Errorf("reading js file: %v", err)
	}
	chartHTML := strings.Replace(htmlContent, "REPLACE_ME_WITH_CHART_JS", jsIntro+string(jsFile), 1)
	err = ioutil.WriteFile("../mainapp/static/dev-chart.html", []byte(chartHTML), 0644)
	if err != nil {
		return []byte{}, fmt.Errorf("writing file: %v", err)
	}
	fmt.Println("Wrote file: https://waterloo:8100/dev-chart.html")

	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 75*time.Second)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(ctx, saveSVGAsPNG(chartHTML, &buf)); err != nil {
		return []byte{}, fmt.Errorf("running chromedp: %v", err)
	}
	return buf, nil
}

func saveSVGAsPNG(htmlContent string, buf *[]byte) chromedp.Tasks {
	dataURL := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(htmlContent))

	return chromedp.Tasks{
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible(`svg`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Set the viewport to match the SVG size
			err := emulation.SetDeviceMetricsOverride(675, 750, 1, false).
				WithScreenOrientation(&emulation.ScreenOrientation{
					Type:  emulation.OrientationTypePortraitPrimary,
					Angle: 0,
				}).
				Do(ctx)
			if err != nil {
				return err
			}

			// Capture the screenshot as PNG
			*buf, err = page.CaptureScreenshot().
				WithQuality(90).
				WithClip(&page.Viewport{
					X:      0,
					Y:      0,
					Width:  675,
					Height: 750,
					Scale:  1,
				}).
				Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
	}
}
