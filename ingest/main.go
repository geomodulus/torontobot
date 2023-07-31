package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
	//"github.com/geomodulus/torontobot/opendata"
)

var operatingBudgetFiles = map[int]string{
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

var serviceRequestsFiles = map[int]string{
	2023: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/079766f3-815d-4257-8731-5ff6b0c84c13/download/sr2023.zip",
	2022: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/f00a3313-f074-463e-89a7-26563084fbef/download/sr2022.zip",
	2021: "https://www.toronto.ca/data/311/opendata/servicerequest/SR2021.zip",
	2020: "https://www.toronto.ca/data/311/opendata/servicerequest/SR2020.zip",
	2019: "https://www.toronto.ca/data/311/opendata/servicerequest/SR2019.zip",
	2018: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/5cce361d-35af-4251-802e-1b1ea1306a07/download/311-service-requests-2018.zip",
	2017: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/0b45485e-690d-425b-a69b-8b8c4f039f2b/download/311-service-requests-2017.zip",
	2016: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/4c6f5bed-5e7f-41c9-95c0-7181e048cdcf/download/311-service-requests-2016.zip",
	2015: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/e33b6300-8899-4f73-8ed0-febe10cbce92/download/311-service-requests-2015.zip",
	2014: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/de039d83-15fe-4a22-ad84-b0389a8c04d9/download/311-service-requests-2014.zip",
	2013: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/ddd49c44-e32f-4b9c-8053-39e0398ae665/download/311-service-requests-2013.zip",
	2012: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/5f0fd6dc-401d-422d-965d-f55cc98ec3f1/download/311-service-requests-2012.zip",
	2011: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/c98df60e-530c-4e3d-a0c9-d21f0af350c9/download/311-service-requests-2011.zip",
	2010: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/2e54bc0e-4399-4076-b717-351df5918ae7/resource/e256aa93-1c8e-4b2e-ba63-a269081b3f55/download/311-service-requests-2010.zip",
}

type kv struct {
	Key   string
	Value int
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

	for year, url := range operatingBudgetFiles {
		if err := processOperatingBudgetYear(db, year, url); err != nil {
			log.Fatalf("Error processing %d: %v", year, err)
		}
	}

	for year, url := range serviceRequestsFiles {
		if err := process311ServiceRequestsYear(db, year, url); err != nil {
			log.Fatalf("Error processing %d: %v", year, err)
		}
	}
}

func processOperatingBudgetYear(db *sql.DB, year int, url string) error {
	// Prepare the SQL statement for inserting datka
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

var (
	parksForestryRecreationFix = &Correction{
		ToReplace:   "Parks, Forestry & Recreation",
		ReplaceWith: `"Parks, Forestry & Recreation"`,
	}
	transferDisplosalOperationsFix = &Correction{
		ToReplace:   "Transfer, Disposal & Operations",
		ReplaceWith: `"Transfer, Disposal & Operations"`,
	}

	corrections = map[int][]*Correction{
		2023: {parksForestryRecreationFix, transferDisplosalOperationsFix},
		2022: {parksForestryRecreationFix, transferDisplosalOperationsFix},
		2021: {transferDisplosalOperationsFix},
	}
)

func process311ServiceRequestsYear(db *sql.DB, year int, url string) error {
	records, err := FetchZipAndParseCSV(url, corrections[year])
	if err != nil {
		return fmt.Errorf("failed to fetch and parse CSV: %v", err)
	}

	// Prepare the SQL statement for inserting data
	stmt, err := db.Prepare(`
INSERT INTO service_requests
(creation_date, status, postal_code_prefix, ward, service_request_type, division, section, year)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	log.Println("for year", year, "importing", len(records), "records")

	for i, record := range records {
		var (
			creationDate       time.Time
			status             = normalizeStatus(record[1])
			postalPrefix       = record[2]
			ward               = record[5]
			serviceRequestType = record[6]
			division           = record[7]
			section            = record[8]
			err                error
		)

		creationDate, err = time.Parse("2006-01-02 15:04:05.0000000", record[0])
		if err != nil {
			return fmt.Errorf("failed to parse creation date in row %d: %v", i+1, err)
		}

		// Execute the prepared statement with the row data
		if _, err = stmt.Exec(
			creationDate,
			status,
			postalPrefix,
			ward,
			serviceRequestType,
			division,
			section,
			year); err != nil {
			return fmt.Errorf("failed to insert data in row %+v: %v", record, err)
		}
	}

	fmt.Printf("%d 311 Service Requests - Customer Initiated imported.\n", year)

	return nil
}

type Correction struct {
	ToReplace   string
	ReplaceWith string
}

// FetchZipAndParseCSV takes a URL pointing to a .zip file containing a CSV,
// downloads and unzips it in memory, then parses and returns the CSV data.
func FetchZipAndParseCSV(url string, corrections []*Correction) ([][]string, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error getting zip file: %w", err)
	}
	defer res.Body.Close()

	bodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading zip file: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("error reading zip file: %w", err)
	}

	rc, err := zipReader.File[0].Open()
	if err != nil {
		return nil, fmt.Errorf("error opening file from zip: %w", err)
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	var correctedData strings.Builder
	var isFirstLine = true
	var headers []string

	for scanner.Scan() {
		line := scanner.Text()
		for _, correction := range corrections {
			line = strings.ReplaceAll(line, correction.ToReplace, correction.ReplaceWith)
		}

		if isFirstLine {
			headers = strings.Split(line, ",")
			for i, header := range headers {
				if strings.HasPrefix(header, `"`) && strings.HasSuffix(header, `"`) {
					headers[i] = strings.Trim(header, `"`)
				}
			}
			isFirstLine = false
		} else {
			correctedData.WriteString(line + "\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file from zip: %w", err)
	}

	correctedReader := strings.NewReader(correctedData.String())
	csvReader := csv.NewReader(correctedReader)
	//csvReader.FieldsPerRecord = -1

	var records [][]string
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading csv file for %s: %w", url, err)
		}

		records = append(records, record)
	}

	fmt.Printf("Headers (len: %d): [", len(headers))
	for _, header := range headers[:len(headers)-1] {
		fmt.Printf("%q, ", header)
	}
	fmt.Printf("%q]\n", headers[len(headers)-1])
	return records, nil
}

func normalizeStatus(status string) string {
	status = strings.ToLower(status)
	status = strings.TrimSpace(status)
	status = strings.ReplaceAll(status, " ", "-")
	return status
}
