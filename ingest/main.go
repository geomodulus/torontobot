package main

import (
	"database/sql"
	"flag"
	"log"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbFile := flag.String("db-file", "../db/toronto.db", "Database file for tabular city data")
	year := flag.Int("year", 0, "Year to process")
	flag.Parse()

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	switch flag.Arg(0) {
	case "311-service-requests":
		if *year != 0 {
			url, ok := serviceRequestsFiles[*year]
			if !ok {
				log.Fatalf("No data for year %d", *year)
			}
			if err := process311ServiceRequestsYear(db, *year, url); err != nil {
				log.Fatalf("Error processing %d: %v", *year, err)
			}
		} else {
			for year, url := range serviceRequestsFiles {
				if err := process311ServiceRequestsYear(db, year, url); err != nil {
					log.Fatalf("Error processing %d: %v", year, err)
				}
			}
		}
	case "operating-budget":
		if *year != 0 {
			url, ok := operatingBudgetFiles[*year]
			if !ok {
				log.Fatalf("No data for year %d", *year)
			}
			if err := processOperatingBudgetYear(db, *year, url); err != nil {
				log.Fatalf("Error processing %d: %v", *year, err)
			}
		} else {
			for year, url := range operatingBudgetFiles {
				if err := processOperatingBudgetYear(db, year, url); err != nil {
					log.Fatalf("Error processing %d: %v", year, err)
				}
			}
		}
	case "ase-tickets":
		if *year != 0 {
			url, ok := aseTicketsFiles[*year]
			if !ok {
				log.Fatalf("No data for year %d", *year)
			}
			if err := processASETicketsYear(db, *year, url); err != nil {
				log.Fatalf("Error processing %d: %v", *year, err)
			}
		} else {
			for year, url := range aseTicketsFiles {
				if err := processASETicketsYear(db, year, url); err != nil {
					log.Fatalf("Error processing %d: %v", year, err)
				}
			}
		}
	case "condo-apartment-price":
		if *year != 0 {
			url, ok := condoApartmentFiles[*year]
			if !ok {
				log.Fatalf("No data for year %d", *year)
			}
			if err := processCondoApartmentPriceYear(db, *year, url); err != nil {
				log.Fatalf("Error processing %d: %v", *year, err)
			}
		} else {
			for year, url := range condoApartmentFiles {
				if err := processCondoApartmentPriceYear(db, year, url); err != nil {
					log.Fatalf("Error processing %d: %v", year, err)
				}
			}
		}

	case "all":
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

		for year, url := range aseTicketsFiles {
			if err := processASETicketsYear(db, year, url); err != nil {
				log.Fatalf("Error processing %d: %v", year, err)
			}
		}

		for year, url := range condoApartmentFiles {
			if err := processCondoApartmentPriceYear(db, year, url); err != nil {
				log.Fatalf("Error processing %d: %v", year, err)
			}
		}
	default:
		log.Fatal(`# TorontoBot Ingest

There are two supported datasets: 
  1. 311-service-requests
  2. operating-budget
  3. ase-tickets
  4. condo-apartment-price

To ingest either one, pass the dataset name as an argument to this program. For example:
  ./ingest 311-service-requests

You can scope to a single year by passing the --year flag. For example:
  ./ingest --year 2017 311-service-requests

You can ingest all years for all datasets (warning: takes a while) by running:
  ./ingest all
`)
	}
}
