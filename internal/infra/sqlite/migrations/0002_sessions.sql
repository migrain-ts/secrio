CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    blob       BLOB    NOT NULL,
    expires_at INTEGER NOT NULL,
    last_used  INTEGER NOT NULL,
    idle_ms    INTEGER NOT NULL
);
