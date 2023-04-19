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
)

type Response struct {
	Schema        string
	Applicability string
	SQL           string
}

func main() {
	dbFile := flag.String("db-file", "./db/toronto.db", "Database file for tabular city data")
	openaiToken := flag.String("openai-token", "", "Token for accessing OpenAI API")
	flag.Parse()

	p := message.NewPrinter(language.English)

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

	// Init AI agent here
	rl, err := readline.New(">> ")
	if err != nil {
		log.Fatal(err)
	}
	// loop to read commands and print output
	for {
		command, err := rl.Readline()

		var query bytes.Buffer
		if err != nil {
			break
		}
		data := struct {
			Command string
		}{
			Command: command,
		}
		if err := prompt.Execute(&query, data); err != nil {
			log.Fatal("Error executing template:", err)
		}
		fmt.Println("% sending request to openai...")
		aiResp, err := ai.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
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

		var resp Response
		if err := json.Unmarshal([]byte(aiResp.Choices[0].Message.Content), &resp); err != nil {
			log.Fatalf("Error unmarshalling response %q: %v", aiResp.Choices[0].Message.Content, err)
		}
		fmt.Printf("Torontobot: %s\n\n%s\n\nExecuting query %q\n", resp.Schema, resp.Applicability, resp.SQL)

		rows, err := db.Query(resp.SQL)
		if err != nil {
			log.Fatalf("Query: %v", err)
		}
		defer rows.Close()

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

		fmt.Println("\nQuery result:")
		fmt.Println(tw.Render())
		fmt.Println()
	}
}
