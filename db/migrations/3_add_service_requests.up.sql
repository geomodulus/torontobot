CREATE TABLE service_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    year INTEGER,
    status TEXT CHECK( status IN ('cancelled', 'closed', 'completed', 'in-progress', 'initiated', 'new', 'unknown') ),
    postal_code_prefix TEXT,
    ward TEXT,
    service_request_type TEXT,
    division TEXT,
    section TEXT,
    creation_date DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
