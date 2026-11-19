// Package sqlite implements app.RecordStore on a single SQLite file.
//
// Concurrency: each Store owns a database/sql pool limited to one
// connection, so calls on one Store are serialised in-process. Write
// transactions use BEGIN IMMEDIATE (_txlock=immediate) and busy_timeout(5000)
// absorbs contention between Stores/processes on the same file. Inside Tx the
// pool's only connection belongs to the transaction, so fn must use the
// tx-bound store it is given, never the outer one.
//
// Timestamps are stored as unix milliseconds (UTC) integers.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
)

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store implements app.RecordStore.
type Store struct {
	db *sql.DB
	q  querier
	tx bool
}

var _ app.RecordStore = (*Store)(nil)

// Close releases the underlying database handle. On the tx-bound store given
// to a Tx callback it is a no-op: the pool belongs to the outer store.
func (s *Store) Close() error {
	if s.tx {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Meta(ctx context.Context) (app.VaultMeta, error) {
	var m app.VaultMeta
	var created int64
	var threads int
	err := s.q.QueryRowContext(ctx, `SELECT format_version, vault_id, name, kdf_time, kdf_mem_kib, kdf_threads,
		salt, wrapped_vault_key, created_at FROM meta WHERE id = 1`).
		Scan(&m.FormatVersion, &m.ID, &m.Name, &m.KDF.Time, &m.KDF.MemoryKiB, &threads, &m.Salt, &m.WrappedVaultKey, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return app.VaultMeta{}, domain.ErrNotFound
	}
	if err != nil {
		return app.VaultMeta{}, mapErr(err)
	}
	m.KDF.Threads = uint8(threads)
	m.CreatedAt = time.UnixMilli(created).UTC()
	return m, nil
}

func (s *Store) PutMeta(ctx context.Context, m app.VaultMeta) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO meta(id, format_version, vault_id, name, kdf_time, kdf_mem_kib,
		kdf_threads, salt, wrapped_vault_key, created_at) VALUES (1,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET format_version=excluded.format_version, vault_id=excluded.vault_id,
		name=excluded.name, kdf_time=excluded.kdf_time, kdf_mem_kib=excluded.kdf_mem_kib,
		kdf_threads=excluded.kdf_threads, salt=excluded.salt, wrapped_vault_key=excluded.wrapped_vault_key,
		created_at=excluded.created_at`,
		m.FormatVersion, m.ID, m.Name, m.KDF.Time, m.KDF.MemoryKiB, int(m.KDF.Threads),
		m.Salt, m.WrappedVaultKey, m.CreatedAt.UnixMilli())
	return mapErr(err)
}

func (s *Store) Put(ctx context.Context, r app.Record) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO records(kind, id, parent_id, blob, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(kind, id) DO UPDATE SET parent_id=excluded.parent_id, blob=excluded.blob,
		updated_at=excluded.updated_at`,
		string(r.Kind), r.ID, r.ParentID, r.Blob, r.UpdatedAt.UnixMilli())
	return mapErr(err)
}

func (s *Store) Get(ctx context.Context, kind app.Kind, id string) (app.Record, error) {
	row := s.q.QueryRowContext(ctx, `SELECT kind, id, parent_id, blob, updated_at FROM records WHERE kind=? AND id=?`,
		string(kind), id)
	r, err := scanRecord(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return app.Record{}, domain.ErrNotFound
	}
	return r, mapErr(err)
}

func (s *Store) List(ctx context.Context, kind app.Kind, parentID string) ([]app.Record, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT kind, id, parent_id, blob, updated_at FROM records
		WHERE kind=? AND parent_id=? ORDER BY id`, string(kind), parentID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []app.Record
	for rows.Next() {
		r, err := scanRecord(rows.Scan)
		if err != nil {
			return nil, mapErr(err)
		}
		out = append(out, r)
	}
	return out, mapErr(rows.Err())
}

func (s *Store) Delete(ctx context.Context, kind app.Kind, id string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM records WHERE kind=? AND id=?`, string(kind), id)
	return mapErr(err)
}

// Tx runs fn in one transaction: commit on nil, rollback on error or panic.
// It cannot be nested.
func (s *Store) Tx(ctx context.Context, fn func(app.RecordStore) error) (err error) {
	if s.tx {
		return errors.New("sqlite: nested transaction")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mapErr(err)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(&Store{db: s.db, q: tx, tx: true}); err != nil {
		tx.Rollback()
		return err
	}
	return mapErr(tx.Commit())
}

func scanRecord(scan func(...any) error) (app.Record, error) {
	var r app.Record
	var kind string
	var ts int64
	if err := scan(&kind, &r.ID, &r.ParentID, &r.Blob, &ts); err != nil {
		return app.Record{}, err
	}
	r.Kind = app.Kind(kind)
	r.UpdatedAt = time.UnixMilli(ts).UTC()
	return r, nil
}
