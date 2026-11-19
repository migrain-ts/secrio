package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, registers "sqlite"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// Opener implements app.VaultOpener on SQLite files.
type Opener struct{}

var _ app.VaultOpener = Opener{}

// Create makes a NEW vault file (mode 0600, created before SQLite touches
// it). It returns domain.ErrDuplicate if the path already exists.
func (Opener) Create(ctx context.Context, path string) (app.RecordStore, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, domain.ErrDuplicate
		}
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return open(ctx, path)
}

// Open opens an existing vault file and applies pending migrations.
func (Opener) Open(ctx context.Context, path string) (app.RecordStore, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return open(ctx, path)
}

func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	// Write transactions take the write lock up front so that contention is
	// absorbed by busy_timeout instead of failing mid-transaction.
	q.Set("_txlock", "immediate")
	return (&url.URL{Scheme: "file", Opaque: escapePath(path), RawQuery: q.Encode()}).String()
}

func escapePath(p string) string {
	return strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(p)
}

func open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, mapErr(err)
	}
	// One connection per store: Go-side serialisation avoids intra-process
	// SQLITE_BUSY; across stores/processes busy_timeout does the waiting.
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	// SQLite copies the main file's mode to -wal/-shm; enforce 0600 anyway.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Chmod(path+suffix, 0o600); err != nil && !errors.Is(err, fs.ErrNotExist) {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db, q: db}, nil
}

// mapErr translates driver errors to domain errors without echoing driver
// text (which could include file content).
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUnsupportedFormat) {
		return err
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "not a database") || strings.Contains(s, "malformed") || strings.Contains(s, "(26)") || strings.Contains(s, "(11)") {
		return domain.ErrCorrupt
	}
	return err
}
