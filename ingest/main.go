package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

func main() {
	dbFile := flag.String("db-file", "../db/toronto.db", "Database file for tabular city data")
	dataFile := flag.String("data-file", "data/approved-operating-budget-summary-2023.xlsx", "Database file for tabular city data")
	flag.Parse()

	// Open the XLSX file
	file, err := excelize.OpenFile(*dataFile)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Prepare the SQL statement for inserting data
	stmt, err := db.Prepare(`
INSERT INTO operating_budget
(program, service, activity, entry_type, category, subcategory, item, year, amount)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	// Get rows in the first sheet
	rows, err := file.GetRows(file.GetSheetName(1))
	if err != nil {
		log.Fatal(err)
	}

	log.Println("importing", len(rows)-1, "rows")
	// Iterate through the rows, skipping the header row
	for i, row := range rows {
		if i == 0 {
			continue
		}

		// Parse the amount, removing commas and parentheses for negative numbers
		amountStr := strings.ReplaceAll(row[7], ",", "")
		amountStr = strings.ReplaceAll(amountStr, "(", "")
		amountStr = strings.ReplaceAll(amountStr, ")", "")
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			log.Fatalf("Error parsing amount in row %d: %v", i+1, err)
		}

		var entryType string
		switch strings.ToLower(row[3]) {
		case "revenues":
			entryType = "revenue"
			amount = amount * -1 // For our purposes revenue is positive, it's negative in the data.
		case "expenses":
			entryType = "expense"
		}

		// Execute the prepared statement with the row data
		_, err = stmt.Exec(row[0], row[1], row[2], entryType, row[4], row[5], row[6], 2023, amount)
		if err != nil {
			log.Fatalf("Error inserting data in row %d: %v", i+1, err)
		}
	}

	fmt.Println("Operating budget imported successfully.")
}
