package reader

import (
	"database/sql"
	"fmt"
	"log"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/jedib0t/go-pretty/v6/table"
)

func ReadDataTable(db *sql.DB, sqlQuery string, isCurrency bool) (string, error) {
	p := message.NewPrinter(language.English)

	rows, err := db.Query(sqlQuery)
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

	hasRows := false
	for rows.Next() {
		hasRows = true
		columns := make([]interface{}, columnCount)
		columnPointers := make([]interface{}, columnCount)

		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return "", fmt.Errorf("error scanning row: %v", err)
		}

		var prefix string
		if isCurrency {
			prefix = "$"
		}
		row := make(table.Row, columnCount)
		for i, column := range columns {
			switch v := column.(type) {
			case int, int32, int64:
				row[i] = p.Sprintf("%s%d", prefix, v)
			case float32, float64:
				row[i] = p.Sprintf("%s%.2f", prefix, v)
			case string:
				row[i] = v
			case nil:
				row[i] = "<no data found>"
			default:
				return "", fmt.Errorf("do not handle type of: %+v", v)
			}
		}
		tw.AppendRow(row)
	}
	rows.Close()

	if !hasRows {
		return "", sql.ErrNoRows
	}

	return tw.Render(), nil
}
