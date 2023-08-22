CREATE TABLE IF NOT EXISTS condominium_apartment_price (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    record_period  TEXT NOT NULL,
    record_start_month int NOT NULL,
    record_end_month int NOT NULL,
    year INTEGER NOT NULL,
    geolocation TEXT NOT NULL,
    price_index FLOAT NOT NULL
);
