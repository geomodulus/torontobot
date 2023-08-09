package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

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

// Correction represents an update, a raw string replacement, to be applied to the dataset prior
// to parsing. Unfortunately, these datasets sometimes have formatting errors, like missing quotes.
type Correction struct {
	// ToReplace is the string to replace.
	ToReplace string
	// ReplaceWith is what to replace it with.
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
