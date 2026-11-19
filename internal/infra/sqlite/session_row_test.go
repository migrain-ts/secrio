package sqlite_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
	"secrio/internal/infra/sqlite"
)

var t0 = time.UnixMilli(1700000000000)

func sessRow() app.SessionRow {
	return app.SessionRow{Blob: []byte("sealed"), ExpiresAt: t0.Add(time.Hour), LastUsed: t0, Idle: 10 * time.Minute}
}

func TestPutGetSessionRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	if err := s.PutSession(ctx, sessRow()); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := sessRow()
	if string(got.Blob) != "sealed" || !got.ExpiresAt.Equal(w.ExpiresAt) || !got.LastUsed.Equal(w.LastUsed) ||
		got.Idle != w.Idle || got.ExpiresAt.Location() != time.UTC {
		t.Errorf("got %+v", got)
	}
	w.Blob = []byte("v2")
	if err := s.PutSession(ctx, w); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetSession(ctx); string(got.Blob) != "v2" {
		t.Errorf("upsert: %q", got.Blob)
	}
}

func TestPutSessionSubMillisecondIdleStoredAsOne(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	r := sessRow()
	r.Idle = time.Microsecond
	s.PutSession(ctx, r)
	if got, _ := s.GetSession(ctx); got.Idle != time.Millisecond {
		t.Errorf("idle = %v", got.Idle)
	}
	r.Idle = 0
	s.PutSession(ctx, r)
	if got, _ := s.GetSession(ctx); got.Idle != 0 {
		t.Errorf("idle = %v", got.Idle)
	}
}

func TestGetSessionNoneIsLocked(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.GetSession(context.Background()); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("err = %v", err)
	}
}

func TestTouchSessionUpdatesLastUsed(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	s.PutSession(ctx, sessRow())
	now := t0.Add(time.Minute)
	ok, err := s.TouchSession(ctx, now)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got, _ := s.GetSession(ctx); !got.LastUsed.Equal(now) {
		t.Errorf("last_used = %v", got.LastUsed)
	}
}

func TestTouchSessionFalseWhenExpired(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	s.PutSession(ctx, sessRow())
	if ok, err := s.TouchSession(ctx, t0.Add(time.Hour)); err != nil || ok {
		t.Errorf("ok=%v err=%v", ok, err)
	}
}

func TestTouchSessionFalseWhenIdleElapsed(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	s.PutSession(ctx, sessRow())
	if ok, err := s.TouchSession(ctx, t0.Add(10*time.Minute)); err != nil || ok {
		t.Errorf("ok=%v err=%v", ok, err)
	}
	r := sessRow()
	r.Idle = 0
	s.PutSession(ctx, r)
	if ok, err := s.TouchSession(ctx, t0.Add(50*time.Minute)); err != nil || !ok {
		t.Errorf("no idle limit: ok=%v err=%v", ok, err)
	}
}

func TestTouchSessionFalseWhenDeleted(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	if ok, err := s.TouchSession(ctx, t0); err != nil || ok {
		t.Errorf("none: ok=%v err=%v", ok, err)
	}
	s.PutSession(ctx, sessRow())
	s.DeleteSession(ctx)
	if ok, err := s.TouchSession(ctx, t0); err != nil || ok {
		t.Errorf("deleted: ok=%v err=%v", ok, err)
	}
}

func TestDeleteSessionIdempotent(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	for i := 0; i < 2; i++ {
		if err := s.DeleteSession(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.PutSession(ctx, sessRow())
	s.DeleteSession(ctx)
	if _, err := s.GetSession(ctx); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("err = %v", err)
	}
}

func TestTouchVsDeleteAcrossTwoStores(t *testing.T) {
	ctx := context.Background()
	s1, path := newStore(t)
	s2, err := sqlite.Opener{}.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.(io.Closer).Close()
	s1.PutSession(ctx, sessRow())
	s1.DeleteSession(ctx)
	if ok, err := s2.TouchSession(ctx, t0); err != nil || ok {
		t.Fatalf("after delete: ok=%v err=%v", ok, err)
	}

	s1.PutSession(ctx, sessRow())
	var wg sync.WaitGroup
	var mu sync.Mutex
	deleted := false
	bad := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			doneBefore := deleted
			mu.Unlock()
			ok, err := s2.TouchSession(ctx, t0)
			mu.Lock()
			defer mu.Unlock()
			// true after Delete completed before the call started is a bug.
			if err != nil || (doneBefore && ok) {
				bad++
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s1.DeleteSession(ctx); err != nil {
			t.Error(err)
		}
		mu.Lock()
		deleted = true
		mu.Unlock()
	}()
	wg.Wait()
	if bad != 0 {
		t.Errorf("%d touches misbehaved", bad)
	}
	if _, err := s1.GetSession(ctx); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("row not gone: %v", err)
	}
	if ok, _ := s2.TouchSession(ctx, t0); ok {
		t.Error("touch true after delete")
	}
}

func TestSessionMethodsInsideTx(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	err := s.Tx(ctx, func(tx app.RecordStore) error {
		if err := tx.PutSession(ctx, sessRow()); err != nil {
			return err
		}
		if ok, err := tx.TouchSession(ctx, t0.Add(time.Minute)); err != nil || !ok {
			t.Errorf("touch in tx: ok=%v err=%v", ok, err)
		}
		_, err := tx.GetSession(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Tx(ctx, func(tx app.RecordStore) error { return tx.DeleteSession(ctx) }); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err = s.Tx(ctx, func(tx app.RecordStore) error {
		tx.PutSession(ctx, sessRow())
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("rollback: %v", err)
	}
}
