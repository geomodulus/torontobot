package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var aseTicketsFiles = map[int]string{
	2023: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/537923d1-a6c8-4b9c-9d55-fa47d9d7ddab/resource/a388bc08-622c-4647-bad8-ecdb7e62090a/download/Automated%20Speed%20Enforcement%20-%20Monthly%20Charges.xlsx",
	2022: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/537923d1-a6c8-4b9c-9d55-fa47d9d7ddab/resource/a388bc08-622c-4647-bad8-ecdb7e62090a/download/Automated%20Speed%20Enforcement%20-%20Monthly%20Charges.xlsx",
	2021: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/537923d1-a6c8-4b9c-9d55-fa47d9d7ddab/resource/a388bc08-622c-4647-bad8-ecdb7e62090a/download/Automated%20Speed%20Enforcement%20-%20Monthly%20Charges.xlsx",
	2020: "https://ckan0.cf.opendata.inter.prod-toronto.ca/dataset/537923d1-a6c8-4b9c-9d55-fa47d9d7ddab/resource/a388bc08-622c-4647-bad8-ecdb7e62090a/download/Automated%20Speed%20Enforcement%20-%20Monthly%20Charges.xlsx",
}

func processASETicketsYear(db *sql.DB, year int, url string) error {
	// Prepare the SQL statement for inserting datka
	stmt, err := db.Prepare(`
INSERT INTO ase_tickets
(site_code, location, enforcement_start_date, enforcement_end_date, month, year, ticket_count, estimated_fine)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
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

	// Get rows in the sheet with name 'Charges by Site and Month', as it is the only sheet in the file.
	useSheet := "Charges by Site and Month"

	rows, err := file.GetRows(useSheet)

	if err != nil {
		return fmt.Errorf("failed to get rows from XLSX file: %v, have sheetlist %+v", err, file.GetSheetList())
	}

	log.Println("for year", year, "importing", len(rows)-1, "rows")

	// Iterate through the rows, skipping the header row
	var siteCodeIdx, locationIdx, enforcementStartDateIdx, enforcementEndDateIdx = -1, -1, -1, -1
	ticketNumberIdxMap := make(map[int]int)
	estimated_avg_fine_per_ticket := 180 //NOTE: this is an estimated value

	for i, row := range rows {
		if i == 0 {
			for j, col := range row {
				switch strings.ToLower(strings.TrimSpace(col)) {
				case "site code":
					siteCodeIdx = j
				case "location*":
					locationIdx = j
				case "enforcement start date":
					enforcementStartDateIdx = j
				case "enforcement end date":
					enforcementEndDateIdx = j
				default:
					monthYear, err := time.Parse("Jan-06", strings.TrimSpace(col))
					if err == nil {
						if monthYear.Year() == year {
							ticketNumberIdxMap[int(monthYear.Month())] = j
						}
					} else {
						fmt.Printf("unknown column %q with error %s\n", col, err)
					}
				}
			}

			continue
		}

		if len(row) < 5 { // total of >5: site_code,location,enforcement_start_date,enforcement_end_date,ticket_number
			continue
		}

		if siteCodeIdx == -1 || locationIdx == -1 || enforcementStartDateIdx == -1 || enforcementEndDateIdx == -1 {
			continue
		}

		var site_code, location string

		if siteCodeIdx != -1 {
			site_code = strings.TrimSpace(row[siteCodeIdx])
		}

		if locationIdx != -1 {
			location = strings.TrimSpace(row[locationIdx])
		}

		var enforcement_start_date, enforcement_end_date string
		var enforcementStartDateTime, enforcementEndDateTime time.Time

		if enforcementStartDateIdx != -1 {
			dateStr := strings.TrimSpace(row[enforcementStartDateIdx])
			enforcement_start_date = dateStr
			if dateStr == "Present" {
				enforcementStartDateTime = time.Now()
			} else {
				date, err := time.Parse("2-Jan-06", dateStr)
				if err != nil {
					fmt.Printf("unknown date format %s with error %s\n", dateStr, err)
					continue
				}
				enforcementStartDateTime = date
			}
		}

		if enforcementEndDateIdx != -1 {
			dateStr := strings.TrimSpace(row[enforcementEndDateIdx])
			enforcement_end_date = dateStr
			if dateStr == "Present" {
				enforcementEndDateTime = time.Now()
			} else {
				date, err := time.Parse("2-Jan-06", dateStr)
				if err != nil {
					fmt.Printf("unknown date format %s with error %s\n", dateStr, err)
					continue
				}
				enforcementEndDateTime = date
			}
		}

		oneMonthLater := enforcementStartDateTime

		for int(oneMonthLater.Month()) <= int(enforcementEndDateTime.Month()) && oneMonthLater.Year() <= enforcementEndDateTime.Year() {
			if oneMonthLater.Year() == year { // skip the data if it is not within the target year
				if ticketNumberIdx, exists := ticketNumberIdxMap[int(oneMonthLater.Month())]; exists {

					ticketNumberStr := strings.ReplaceAll(strings.TrimSpace(row[ticketNumberIdx]), ",", "")
					ticket_number, err := strconv.Atoi(ticketNumberStr)

					if err == nil {
						month := int(oneMonthLater.Month())
						estimated_fine := ticket_number * estimated_avg_fine_per_ticket

						// Execute the prepared statement with the row data
						if _, err = stmt.Exec(
							site_code,
							location,
							enforcement_start_date,
							enforcement_end_date,
							month,
							year,
							ticket_number,
							estimated_fine); err != nil {
							return fmt.Errorf("failed to insert data in row %d: %v", i+1, err)
						}
					}
				}
			}
			oneMonthLater = oneMonthLater.AddDate(0, 1, 0) // fast forward by one month
		}

	}
	fmt.Printf("%d ASE tickets imported.\n", year)

	return nil
}
