package main

import (
	"bytes"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"text/template"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chzyer/readline"
	"github.com/sashabaranov/go-openai"
)

const (
	// Model is the openai model to query. GPT-4 is expensive, so we use GPT-3.5.
	Model = openai.GPT3Dot5Turbo
	// RespTemp is the response temperature we want from the model. Default temp is 1.0 and higher
	// is more "creative".
	RespTemp = 0.5
)

func main() {
	dbFile := flag.String("db-file", "./db/toronto.db", "Database file for tabular city data")
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
		resp, err := ai.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
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
		fmt.Printf("Torontobot: %+v", resp.Choices[0].Message.Content)
	}
}
