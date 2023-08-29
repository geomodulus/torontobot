package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var condoApartmentFiles = map[int]string{
	2023: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20230101&endDate=20231001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
	2022: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20220101&endDate=20221001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
	2021: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20210101&endDate=20211001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
	2020: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20200101&endDate=20201001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
	2019: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20190101&endDate=20191001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
	2018: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20180101&endDate=20181001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
	2017: "https://www150.statcan.gc.ca/t1/tbl1/en/dtl!downloadDbLoadingData-nonTraduit.action?pid=1810027302&latestN=0&startDate=20170101&endDate=20171001&csvLocale=en&selectedMembers=%5B%5B%5D%5D&checkedLevels=0D1",
}

func convertToQuarter(dateTime time.Time) (string, error) {
	switch int(dateTime.Month()) {
	case 1:
		return "Q1", nil
	case 4:
		return "Q2", nil
	case 7:
		return "Q3", nil
	case 10:
		return "Q4", nil
	default:
		return "", fmt.Errorf("failed to convert date time to quarters")
	}
}

func processCondoApartmentPriceYear(db *sql.DB, year int, url string) error {

	// Prepare the SQL statement for inserting datka
	stmt, err := db.Prepare(`
INSERT INTO condominium_apartment_price
(record_period, record_start_month, record_end_month, year, geolocation, price_index)
VALUES (?, ?, ?, ?, ?, ?)
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

	// Fetch attachment
	contentDisposition := resp.Header.Get("Content-Disposition")
	filename := "downloaded.csv"
	if strings.HasPrefix(contentDisposition, "attachment;filename=\"") {
		filename = strings.TrimPrefix(contentDisposition, "attachment;filename=\"")
		filename = strings.TrimSuffix(filename, "\"")
	}

	// Create file locally
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error:", err)
		return fmt.Errorf("failed to create local csv file: %v", err)
	}
	defer file.Close()

	// Copy the CSV content from response body to the local file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Println("Error:", err)
		return fmt.Errorf("failed to copy to CSV file: %v", err)
	}

	// Open the CSV file
	f, err := os.Open(filename)
	csvReader := csv.NewReader(f)
	csvReader.LazyQuotes = true

	// Read the CSV file
	records, err := csvReader.ReadAll()

	if err != nil {
		return fmt.Errorf("failed to open CSV file: %v", err)
	}

	// Iterate through the rows, skipping the header row
	// record_period, record_start_month, record_end_month, year, geolocation, price_index
	var recordStartMonthIdx, geolocationIdx, condoApartmentPriceValueIdx = -1, -1, -1
	// "REF_DATE","GEO","DGUID","UOM","UOM_ID","SCALAR_FACTOR","SCALAR_ID","VECTOR","COORDINATE","VALUE","STATUS","SYMBOL","TERMINATED","DECIMALS"
	for i, row := range records {

		if i == 0 {
			for j, col := range row {

				switch strings.ToLower(strings.TrimSpace(col)) {
				case "geo":
					geolocationIdx = j
				case "value":
					condoApartmentPriceValueIdx = j
				default:
					if strings.Contains(strings.ToLower(strings.TrimSpace(col)), "ref_date") {
						recordStartMonthIdx = j
					}
				}
			}

			continue
		}

		if len(row) < 3 { // total of >6: record_period, geolocation, price_index
			continue
		}

		if recordStartMonthIdx == -1 || geolocationIdx == -1 || condoApartmentPriceValueIdx == -1 {
			continue
		}

		var record_period, geolocation string
		var record_start_month, record_end_month, year int
		var price_index float32

		if recordStartMonthIdx != -1 {
			dateTime, err := time.Parse("2006-01", strings.TrimSpace(row[recordStartMonthIdx]))
			if err == nil {
				quarter, err := convertToQuarter(dateTime)
				if err == nil {
					record_period = quarter
				} else {
					fmt.Printf("cannot convert date time %s to quarter, failed with error %s\n", dateTime, err)
					continue
				}
				record_start_month = int(dateTime.Month())
				record_end_month = int(dateTime.Month()) + 2
				year = dateTime.Year()
			} else {
				fmt.Printf("unknown date format %s with error %s\n", strings.TrimSpace(row[recordStartMonthIdx]), err)
				continue
			}
		}

		if geolocationIdx != -1 {
			geolocation = strings.TrimSpace(row[geolocationIdx])
		}

		if condoApartmentPriceValueIdx != -1 {
			value, err := strconv.ParseFloat(row[condoApartmentPriceValueIdx], 32)
			if err != nil {
				fmt.Printf("cannot convert condo apartment price value %s to integer with error %s\n", row[condoApartmentPriceValueIdx], err)
				continue
			}
			price_index = float32(value)

		}

		// fmt.Println("add r", record_period, record_start_month, record_end_month, geolocation, year, price_index)

		// Execute the prepared statement with the row data
		if _, err = stmt.Exec(
			record_period,
			record_start_month,
			record_end_month,
			year,
			geolocation,
			price_index); err != nil {
			return fmt.Errorf("failed to insert data in row %d: %v", i+1, err)
		}
	}

	// Delete the CSV file after use
	e := os.Remove(filename)
	if e != nil {
		return fmt.Errorf("failed to delete CSV file: %v", err)
	}

	fmt.Printf("%d condo apartment price imported.\n", year)

	return nil
}
