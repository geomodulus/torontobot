-- Create a new table with the desired structure
CREATE TABLE new_ase_tickets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_code TEXT NOT NULL,
    location TEXT NOT NULL,
    enforcement_start_date TEXT NOT NULL,
    enforcement_end_date TEXT NOT NULL,
    month INTEGER NOT NULL,
    year INTEGER NOT NULL,
    ticket_count INTEGER NOT NULL, -- Renamed from ticket_number
    estimated_fine INTEGER NOT NULL
);

-- Copy all data from the old table to the new one
INSERT INTO new_ase_tickets (id, site_code, location, enforcement_start_date, enforcement_end_date, month, year, ticket_count, estimated_fine)
SELECT id, site_code, location, enforcement_start_date, enforcement_end_date, month, year, ticket_number, estimated_fine
FROM ase_tickets;

-- Drop the old table
DROP TABLE ase_tickets;

-- Rename the new table to the old table's name
ALTER TABLE new_ase_tickets RENAME TO ase_tickets;

