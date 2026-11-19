CREATE TABLE meta (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    format_version    INTEGER NOT NULL,
    vault_id          TEXT    NOT NULL,
    name              TEXT    NOT NULL,
    kdf_time          INTEGER NOT NULL,
    kdf_mem_kib       INTEGER NOT NULL,
    kdf_threads       INTEGER NOT NULL,
    salt              BLOB    NOT NULL,
    wrapped_vault_key BLOB    NOT NULL,
    created_at        INTEGER NOT NULL
);

CREATE TABLE records (
    kind       TEXT    NOT NULL,
    id         TEXT    NOT NULL,
    parent_id  TEXT    NOT NULL DEFAULT '',
    blob       BLOB    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (kind, id)
);

CREATE INDEX records_kind_parent ON records (kind, parent_id);
