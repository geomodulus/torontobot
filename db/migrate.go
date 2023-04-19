package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	dbFile := flag.String("db", "toronto.db", "SQLite3 database file")
	migrationPath := flag.String("migrations", "./migrations", "Path to migrations")

	flag.Parse()

	dbPath := fmt.Sprintf("sqlite3://%s?cache=shared&mode=rwc", *dbFile)

	if err := applyMigrations(dbPath, *migrationPath); err != nil {
		log.Fatalf("Failed to apply migrations: %v", err)
	}
}

func applyMigrations(dbPath string, migrationPath string) error {
	migrationFullPath := fmt.Sprintf("file://%s", migrationPath)

	m, err := migrate.New(migrationFullPath, dbPath)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
