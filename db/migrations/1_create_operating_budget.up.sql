CREATE TABLE IF NOT EXISTS operating_budget (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    program TEXT NOT NULL,
    service TEXT NOT NULL,
    activity TEXT NOT NULL,
    entry_type TEXT NOT NULL CHECK (entry_type IN ('revenue', 'expense')),
    category TEXT NOT NULL,
    subcategory TEXT,
    item TEXT,
    year INTEGER NOT NULL,
    amount REAL NOT NULL
);
