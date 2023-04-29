package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

var dataFiles = map[int]string{
	2023: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/a6f7a8e8-e497-4f77-9881-daba429ea981/download/approved-operating-budget-summary-2023.xlsx",
	2022: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/9e5f9a63-fdeb-46e4-9f5f-8143038de56d/download/approved-operating-budget-summary-2022.xlsx",
	2021: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/930502fa-87f1-4d71-8b71-4435df594b38/download/approved-operating-budget-summary-2021.xlsx",
	2020: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/4e58558a-4773-4cd0-a16a-de481f25cb4d/download/approved-operating-budget-summary-2020.xlsx",
	2019: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/27bc496e-1507-4d2e-b9f8-4392b1d44b5c/download/approved-operating-budget-summary-2019.xlsx",
	2018: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/8ff2e379-2bbf-43ab-83eb-2caa7717d85b/download/approved-operating-budget-summary-2018.xlsx",
	2017: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/393dffb4-2f7a-4ff3-8443-e44796f08782/download/approved-operating-budget-summary-2017.xlsx",
	2016: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/d55a2458-f116-456e-a3be-4a0d867fa190/download/approved-operating-budget-summary-2016.xlsx",
	2015: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/4a87ff46-996b-4768-b833-69e023b1b4c4/download/approved-operating-budget-summary-2015.xlsx",
	2014: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2c90a5d3-5598-4c02-abf2-169456c8f1f1/resource/7e23b244-91a7-4e70-9baf-be231ff58d59/download/approved-operating-budget-summary-2014.xlsx",
}

func main() {
	dbFile := flag.String("db-file", "../db/toronto.db", "Database file for tabular city data")
	flag.Parse()

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	for year, url := range dataFiles {
		if err := processYear(db, year, url); err != nil {
			log.Fatalf("Error processing %d: %v", year, err)
		}
	}
}

func processYear(db *sql.DB, year int, url string) error {
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

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %d %s", resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()

	// Open the XLSX file
	file, err := excelize.OpenReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to open XLSX file: %v", err)
	}

	// Get rows in the sheet with year as name.
	useSheet := "Open Data"
	for _, sheet := range file.GetSheetList() {
		if sheet == strconv.Itoa(year) ||
			strings.ToLower(sheet) == "summary" ||
			sheet == "Open Data Summary" {
			useSheet = sheet
			break
		}
	}
	rows, err := file.GetRows(useSheet)
	if err != nil {
		return fmt.Errorf("failed to get rows from XLSX file: %v, have sheetlist %+v", err, file.GetSheetList())
	}

	log.Println("for year", year, "importing", len(rows)-1, "rows")
	// Iterate through the rows, skipping the header row
	var programIdx, serviceIdx, activityIdx, entryTypeIdx, categoryIdx, subcategoryIdx, itemIdx, amountIdx = -1, -1, -1, -1, -1, -1, -1, -1
	for i, row := range rows {
		if i == 0 {
			for j, col := range row {
				switch strings.ToLower(strings.TrimSpace(col)) {
				case "program":
					programIdx = j
				case "service":
					serviceIdx = j
				case "activity":
					activityIdx = j
				case "expense/revenue":
					entryTypeIdx = j
				case "category name":
					categoryIdx = j
				case "sub-category name":
					subcategoryIdx = j
				case "commitment item":
					itemIdx = j
				case strconv.Itoa(year):
					amountIdx = j
				default:
					fmt.Printf("unknown column %q\n", col)
				}
			}
			continue
		}
		if row[programIdx] == "0" {
			continue
		}
		if programIdx == -1 || entryTypeIdx == -1 || amountIdx == -1 {
			continue
		}
		program := strings.TrimSpace(row[programIdx])
		var service, activity, entryType, category, subcategory, item string

		if serviceIdx != -1 {
			service = strings.TrimSpace(row[serviceIdx])
		}
		if activityIdx != -1 {
			activity = strings.TrimSpace(row[activityIdx])
		}
		if categoryIdx != -1 {
			category = strings.TrimSpace(row[categoryIdx])
		}
		if subcategoryIdx != -1 {
			subcategory = strings.TrimSpace(row[subcategoryIdx])
		}
		if itemIdx != -1 {
			item = strings.TrimSpace(row[itemIdx])
		}

		// Parse the amount, removing commas and parentheses for negative numbers
		amountStr := strings.ReplaceAll(row[amountIdx], ",", "")
		amountStr = strings.ReplaceAll(amountStr, "(", "")
		amountStr = strings.ReplaceAll(amountStr, ")", "")
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			return fmt.Errorf("failed to parse amount in row %d at idx %d: %v\n%+v", i+1, amountIdx, err, row)
		}

		switch strings.ToLower(row[entryTypeIdx]) {
		case "revenues":
			entryType = "revenue"
			amount = amount * -1 // For our purposes revenue is positive, it's negative in the data.
		case "expenses":
			entryType = "expense"
		default:
			return fmt.Errorf("unknown entry type in row %d (expected revenues or expenses): %s\n%+v", i+1, row[entryTypeIdx], row)
		}

		// Execute the prepared statement with the row data
		if _, err = stmt.Exec(
			program,
			service,
			activity,
			entryType,
			category,
			subcategory,
			item,
			year,
			amount); err != nil {
			return fmt.Errorf("failed to insert data in row %d: %v", i+1, err)
		}
	}
	fmt.Printf("%d operating budget imported.\n", year)

	return nil
}
