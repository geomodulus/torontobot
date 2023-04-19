
CREATE TABLE IF NOT EXISTS operating_budget (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    program TEXT NOT NULL,
    service TEXT NOT NULL,
    activity TEXT,
    entry_type TEXT NOT NULL CHECK (entry_type IN ('revenue', 'expense')),
    category TEXT NOT NULL,
    subcategory TEXT NOT NULL,
    item TEXT NOT NULL,
    year INTEGER NOT NULL,
    amount REAL NOT NULL
);
