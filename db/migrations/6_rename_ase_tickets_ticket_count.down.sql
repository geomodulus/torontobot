-- Create a new table with the original structure
CREATE TABLE old_ase_tickets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_code TEXT NOT NULL,
    location TEXT NOT NULL,
    enforcement_start_date TEXT NOT NULL,
    enforcement_end_date TEXT NOT NULL,
    month INTEGER NOT NULL,
    year INTEGER NOT NULL,
    ticket_number INTEGER NOT NULL, -- Renamed back to ticket_number
    estimated_fine INTEGER NOT NULL
);

-- Copy all data from the current table to the old structure
INSERT INTO old_ase_tickets (id, site_code, location, enforcement_start_date, enforcement_end_date, month, year, ticket_number, estimated_fine)
SELECT id, site_code, location, enforcement_start_date, enforcement_end_date, month, year, ticket_count, estimated_fine
FROM ase_tickets;

-- Drop the current table
DROP TABLE ase_tickets;

-- Rename the old table to the current table's name
ALTER TABLE old_ase_tickets RENAME TO ase_tickets;

