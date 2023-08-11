CREATE TABLE IF NOT EXISTS ase_tickets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site_code TEXT NOT NULL,
    location TEXT NOT NULL,
    enforcement_start_date TEXT NOT NULL,
    enforcement_end_date TEXT NOT NULL,
    month INTEGER NOT NULL,
    year INTEGER NOT NULL,
    ticket_number INTEGER NOT NULL,
    estimated_fine INTEGER NOT NULL
);
