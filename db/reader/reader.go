package reader

import (
	"database/sql"
	"fmt"
	"log"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/jedib0t/go-pretty/v6/table"
)

func ReadDataTable(db *sql.DB, sqlQuery string) (string, error) {
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

	if !hasRows {
		return "", sql.ErrNoRows
	}

	return tw.Render(), nil
}
