package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// PutSession upserts the single session row. A positive Idle below 1 ms is
// stored as 1 ms; 0 means no idle limit.
func (s *Store) PutSession(ctx context.Context, r app.SessionRow) error {
	idle := r.Idle.Milliseconds()
	if r.Idle > 0 && idle < 1 {
		idle = 1
	}
	_, err := s.q.ExecContext(ctx, `INSERT INTO sessions(id, blob, expires_at, last_used, idle_ms) VALUES (1,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET blob=excluded.blob, expires_at=excluded.expires_at,
		last_used=excluded.last_used, idle_ms=excluded.idle_ms`,
		r.Blob, r.ExpiresAt.UnixMilli(), r.LastUsed.UnixMilli(), idle)
	return mapErr(err)
}

// GetSession returns domain.ErrLocked when no session row exists.
func (s *Store) GetSession(ctx context.Context) (app.SessionRow, error) {
	var r app.SessionRow
	var exp, used, idle int64
	err := s.q.QueryRowContext(ctx, `SELECT blob, expires_at, last_used, idle_ms FROM sessions WHERE id=1`).
		Scan(&r.Blob, &exp, &used, &idle)
	if errors.Is(err, sql.ErrNoRows) {
		return app.SessionRow{}, domain.ErrLocked
	}
	if err != nil {
		return app.SessionRow{}, mapErr(err)
	}
	r.ExpiresAt = time.UnixMilli(exp).UTC()
	r.LastUsed = time.UnixMilli(used).UTC()
	r.Idle = time.Duration(idle) * time.Millisecond
	return r, nil
}

// TouchSession is a single atomic statement, so a concurrent DeleteSession
// from another Store or process can never be followed by a successful touch.
func (s *Store) TouchSession(ctx context.Context, now time.Time) (bool, error) {
	ms := now.UnixMilli()
	res, err := s.q.ExecContext(ctx, `UPDATE sessions SET last_used=? WHERE id=1 AND expires_at>?
		AND (idle_ms=0 OR ?-last_used<idle_ms)`, ms, ms, ms)
	if err != nil {
		return false, mapErr(err)
	}
	n, err := res.RowsAffected()
	return n == 1, mapErr(err)
}

func (s *Store) DeleteSession(ctx context.Context) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE id=1`)
	return mapErr(err)
}
