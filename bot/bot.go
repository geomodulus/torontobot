// Package bot contains the core logic for TorontoBot which is shared across host platforms.
package bot

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/chrisdinn/vector-db/index"
	"github.com/rolldever/go-json5"
	"github.com/sashabaranov/go-openai"

	"github.com/geomodulus/citygraph"
	"github.com/geomodulus/torontobot/db/reader"
	"github.com/geomodulus/torontobot/jsonschema"
	"github.com/geomodulus/torontobot/viz"
)

const (
	// Model is the openai model to query. GPT-4 is expensive, so we use GPT-3.5.
	//Model = openai.GPT4
	Model = openai.GPT3Dot5Turbo
	// RespTemp is the response temperature we want from the model. Default temp is 1.0 and higher
	// is more "creative".
	RespTemp = 0.1
)

var (
	//go:embed prompts/sql_gen.txt
	sqlGenTmpl []byte
	//go:embed prompts/chart_select.txt
	chartSelectTmpl string
	//go:embed tables.json5
	tablesJSON []byte
)

type DataTable struct {
	Name         string                       `json:"name"`
	Desc         string                       `json:"description"`
	Schema       string                       `json:"schema"`
	Enums        map[string][]interface{}     `json:"enums"`
	Hints        map[string]map[string]string `json:"hints"`
	Instructions string                       `json:"instructions"`
}

func (t *DataTable) EmbeddingText() string {
	txt := t.Name + "\n" + t.Desc + "\n"
	txt += "Schema:\n" + t.Schema + "\n"
	if len(t.Enums) > 0 {
		txt += "Enums:\n"
		for k, v := range t.Enums {
			var vals []string
			for _, val := range v {
				switch ev := val.(type) {
				case string:
					vals = append(vals, ev)
				case float64:
					vals = append(vals, strconv.FormatFloat(ev, 'f', -1, 64))
				case int:
					vals = append(vals, strconv.Itoa(ev))
				}
			}
			txt += " - " + k + ": " + strings.Join(vals, ", ") + "\n"
		}
	}
	if len(t.Hints) > 0 {
		txt += "Hints:\n"
		for k, v := range t.Hints {
			txt += " - " + k + ": "
			for k2, v2 := range v {
				txt += k2 + ": " + v2 + ", "
			}
			txt += "\n"
		}
	}
	return txt
}

type MsgTemplate struct {
	Role         string               `json:"role"`
	Name         string               `json:"name"`
	Content      string               `json:"content"`
	FunctionCall *openai.FunctionCall `json:"function_call"`

	tmpl *template.Template
}

func (t *MsgTemplate) Parse() error {
	if t.Content == "" {
		return nil
	}

	tmpl, err := template.New("").Parse(t.Content)
	if err != nil {
		return fmt.Errorf("parsing template: %v", err)
	}
	t.tmpl = tmpl
	return nil
}

type TorontoBot struct {
	Hostname          string
	sqlGenPrompt      *template.Template
	sqlGenTemplates   []*MsgTemplate
	chartSelectPrompt *template.Template
	graphStore        *citygraph.Store
	ai                *openai.Client
	db                *sql.DB
	tables            map[string]*DataTable
	tableIndex        *index.VectorIndex[string]
}

func New(ctx context.Context, db *sql.DB, ai *openai.Client, store *citygraph.Store, host string) (*TorontoBot, error) {
	var tableList []*DataTable
	if err := json5.Unmarshal(tablesJSON, &tableList); err != nil {
		log.Fatalf("Error unmarshalling tables.json5: %s", err)
	}
	tables := map[string]*DataTable{}
	for _, table := range tableList {
		tables[table.Name] = table
	}

	embeddings := []*index.DataPoint[string]{}

	var dimensions int
	for _, table := range tables {
		txt := table.EmbeddingText()

		req := openai.EmbeddingRequestStrings{
			Input: []string{txt},
			Model: openai.AdaEmbeddingV2,
		}
		// Generate embeddings
		resp, err := ai.CreateEmbeddings(ctx, req)
		if err != nil {
			if strings.Contains(err.Error(),"Please try again in 20s") {
				fmt.Printf("API use limit error, sleep for 20s before retry ...")
				time.Sleep(21 * time.Second) // add one more second to ensure the limit won't exceeded again
				respSecondTry, errSecondTry := ai.CreateEmbeddings(ctx, req)
				if errSecondTry != nil {
					log.Fatalf("Error creating embeddings on 2nd attempt: %s", err)
				} else {
					resp = respSecondTry
				}
			} else {
				log.Fatalf("Error creating embeddings: %s", err)
			}			
		}
		if len(resp.Data) == 0 {
			log.Fatalf("No embeddings returned for table: %q", table.Name)
		}
		dimensions = len(resp.Data[0].Embedding)
		var vec []float64
		for _, emb := range resp.Data[0].Embedding {
			vec = append(vec, float64(emb))
		}
		embeddings = append(embeddings, &index.DataPoint[string]{
			ID:        table.Name,
			Embedding: vec,
		})
	}

	tableIndex, err := index.NewVectorIndex(1, dimensions, 2, embeddings, index.NewCosineDistanceMeasure())
	if err != nil {
		log.Fatalf("Error creating index: %s", err)
	}

	tableIndex.Build()

	sqlGenPrompt, err := template.New("sql_gen").Parse(string(sqlGenTmpl))
	if err != nil {
		return nil, fmt.Errorf("parsing prompts/sql_gen.txt: %v", err)
	}
	chartSelectPrompt, err := template.New("chart_select").Parse(chartSelectTmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing prompts/chart_select.txt: %v", err)
	}
	return &TorontoBot{
		Hostname:          host,
		sqlGenPrompt:      sqlGenPrompt,
		chartSelectPrompt: chartSelectPrompt,
		graphStore:        store,
		ai:                ai,
		db:                db,
		tables:            tables,
		tableIndex:        tableIndex,
	}, nil
}

func (b *TorontoBot) SelectTable(ctx context.Context, question string) (*DataTable, error) {
	req := openai.EmbeddingRequestStrings{
		Input: []string{question},
		Model: openai.AdaEmbeddingV2,
	}
	// Generate embeddings
	resp, err := b.ai.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating embeddings: %v", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	var vec []float64
	for _, emb := range resp.Data[0].Embedding {
		vec = append(vec, float64(emb))
	}

	searchResults, err := b.tableIndex.SearchByVector(vec, 2, 10.0)
	if err != nil {
		return nil, fmt.Errorf("searching index: %v", err)
	}
	selected := b.tables[(*searchResults)[0].ID]
	for _, searchResult := range *searchResults {
		fmt.Printf("id: %v, distance: %f\n", searchResult.ID, searchResult.Distance)
	}
	return selected, nil
}

type SQLResponse struct {
	Schema        string `json:"schema"`
	Applicability string `json:"applicability"`
	SQL           string `json:"sql"`
	IsCurrency    bool   `json:"result_is_currency"`
	MissingData   string `json:"missing_data"`
}

var SQLAnalysisFunction = openai.FunctionDefinition{
	Name:        "sql_analysis",
	Description: "Accepts SQL query analysis derived from user queries.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]*jsonschema.Definition{
			"schema": {
				Type:        jsonschema.String,
				Description: "1 to 2 sentences about which columns from the schema to use.",
			},
			"applicability": {
				Type:        jsonschema.String,
				Description: "1 to 2 sentences about which columns and enums are relevant, or which ones are missing.",
			},
			"sql": {
				Type:        jsonschema.String,
				Description: "A single-line SQL query to run. Remember to escape any special characters",
			},
			"result_is_currency": {
				Type:        jsonschema.Boolean,
				Description: "Whether the result of the query is a currency value.",
			},
		},
		Required: []string{"schema", "applicability", "sql", "result_is_currency"},
	},
}

func (b *TorontoBot) SQLAnalysis(ctx context.Context, table *DataTable, question string) (*SQLResponse, error) {
	data := struct {
		Date  string
		Table *DataTable
	}{
		Date:  time.Now().Format("January 2, 2006"),
		Table: table,
	}

	var systemPrompt bytes.Buffer
	if err := b.sqlGenPrompt.Execute(&systemPrompt, data); err != nil {
		return nil, fmt.Errorf("executing sql_gen template: %v", err)
	}

	aiResp, err := b.ai.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: Model,
		Messages: []openai.ChatCompletionMessage{{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt.String(),
		}, {
			Role:    openai.ChatMessageRoleUser,
			Content: question,
		}},
		Temperature: RespTemp,
		Functions: []openai.FunctionDefinition{
			SQLAnalysisFunction,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("CreateChatCompletion: %v", err)
	}

	var resp SQLResponse
	if fc := aiResp.Choices[0].Message.FunctionCall; fc != nil {
		log.Printf("Got function call: %s(%q)\n", fc.Name, fc.Arguments)

		if err := json5.Unmarshal([]byte(fc.Arguments), &resp); err != nil {
			return nil, fmt.Errorf("unmarshalling response %q: %v", aiResp.Choices[0].Message.Content, err)
		}
		// handle function call
	} else {
		resp.MissingData = aiResp.Choices[0].Message.Content
		log.Printf("Got reply text: %s\n", resp.MissingData)
	}

	return &resp, nil
}

func (b *TorontoBot) LoadResults(sqlQuery string, isCurrency bool) (string, error) {
	sqlQuery = sanitizeQuery(sqlQuery)
	fmt.Println("running sqlQuery:", sqlQuery)
	return reader.ReadDataTable(b.db, sqlQuery, isCurrency)
}

type ChartSelectResponse struct {
	Chart           string           `json:"type"`
	Title           string           `json:"title"`
	Data            []*viz.DataEntry `json:"data"`
	ValueIsCurrency bool             `json:"is_currency"`
}

var ChartSelectFunction = openai.FunctionDefinition{
	Name:        "select_chart",
	Description: "Selects a chart type and foramts data to be used in the chart.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]*jsonschema.Definition{
			"type": {
				Type:        jsonschema.String,
				Description: "Selected type of chart for this data.",
				Enum:        []string{"bar", "stacked-bar", "line", "pie"},
			},
			"title": {
				Type:        jsonschema.String,
				Description: "Title for the chart.",
			},
			"data": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]*jsonschema.Definition{
						"name": {
							Type:        jsonschema.String,
							Description: "Name of the data entry.",
						},
						"date": {
							Type:        jsonschema.Number,
							Description: "Year of the data entry.",
						},
						"value": {
							Type:        jsonschema.Number,
							Description: "Value of the data entry.",
						},
					},
					Required: []string{"value"},
				},
			},
			"is_currency": {
				Type:        jsonschema.Boolean,
				Description: "Whether the data value represents money/currency amount or not.",
			},
		},
		Required: []string{"type", "title", "data", "is_currency"},
	},
}

//  Potenial response for stacked bar chart:
//  {
//    "Chart": "stacked bar chart",
//    "Title": TTC vs Police Budgets",
//    "Keys": ["TTC", "Police"],
//    "Data": [{
//      "Date": 2014,
//      "TTC": <item-number>,
//      "Police": <item-number>
//    }, {
//      "Date": 2015,
//      "TTC": <item-number>,
//      "Police": <item-number>
//    }],
//    "ValueIsCurrency": true
//  }

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
		Functions: []openai.FunctionDefinition{
			ChartSelectFunction,
		},
		FunctionCall: map[string]string{
			"name": ChartSelectFunction.Name,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("CreateChatCompletion: %v", err)
	}
	log.Printf("Got reply: %s\n", aiResp.Choices[0].Message.Content)

	fc := aiResp.Choices[0].Message.FunctionCall
	if fc == nil {
		return nil, fmt.Errorf("expected function call in response")
	}
	if fc.Name != ChartSelectFunction.Name {
		return nil, fmt.Errorf("expected function call %q, got %q", ChartSelectFunction.Name, fc.Name)
	}
	var resp ChartSelectResponse
	if err := json.Unmarshal([]byte(fc.Arguments), &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling function call: %v", err)
	}

	fmt.Printf("%+v\n", resp)

	return &resp, nil
}

func (b *TorontoBot) HasGraphStore() bool {
	return b.graphStore != nil
}

func (b *TorontoBot) SaveToGraph(ctx context.Context, id, title, body, chartJS, featureImage, user string) (string, error) {
	camera := map[string]interface{}{
		"": map[string]interface{}{
			"center":  map[string]float64{"lng": -79.3838, "lat": 43.6536},
			"zoom":    16.084,
			"pitch":   0,
			"bearing": -30,
		},
		"md": map[string]interface{}{
			"center": map[string]float64{"lng": -79.3835, "lat": 43.6532},
			"zoom":   16.1558,
		},
	}
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

	chartJS += "\n\nmodule.initAdUnits();\n"
	if err := b.graphStore.WriteJS(ctx, q, viz.RenderGraphJS(chartJS)); err != nil {
		return "", fmt.Errorf("writing JS: %v", err)
	}

	slugID, err := mod.SlugID()
	if err != nil {
		return "", fmt.Errorf("generating slug ID: %v", err)
	}
	return fmt.Sprintf("/mod/%s/%s", slugID, mod.SlugTitle()), nil
}

func sanitizeQuery(sqlQuery string) string {
	// There are some bad phrases GPT-3.5 inserts without escaping, so we need to do it here.
	sqlQuery = strings.ReplaceAll(sqlQuery, "Children's Services", "Children''s Services")
	sqlQuery = strings.ReplaceAll(sqlQuery, "Mayor's Office", "Mayor''s Office")
	return sqlQuery
}
