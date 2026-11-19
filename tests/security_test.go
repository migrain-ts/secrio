package tests

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"

	"secrio/internal/domain"
)

func TestSQLiteFileContainsNoPlaintext(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	u := e.open(t)
	const (
		name = "PLAINTEXT_PROBE_NAME"
		desc = "plaintext-probe-description"
	)
	value := []byte("plaintext-probe-value-0123456789")
	if _, err := u.SetSecret(ctx, e.dev.ID, name, value, "", desc); err != nil {
		t.Fatal(err)
	}
	probes := [][]byte{[]byte(name), value, []byte(desc), []byte("payments-project"),
		[]byte("payments-description"), []byte("dev-env"), []byte("prod-env"), []byte(testPw)}

	scan := func(when string) {
		t.Helper()
		for _, suffix := range []string{"", "-wal", "-shm"} {
			data, err := os.ReadFile(e.dbPath + suffix)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range probes {
				if bytes.Contains(data, p) {
					t.Errorf("%s: %q found in %s", when, p, filepath.Base(e.dbPath)+suffix)
				}
			}
		}
	}
	// Read while the store is open: recent writes live in the -wal file.
	scan("store open")
	if _, err := os.Stat(e.dbPath + "-wal"); err != nil {
		t.Logf("no -wal file while open: %v", err)
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	scan("store closed")

	// The vault name is plaintext by design (registry and meta).
	data, err := os.ReadFile(e.dbPath)
	if err != nil || !bytes.Contains(data, []byte("My Vault")) {
		t.Fatalf("vault name expected in header: %v", err)
	}
}

func TestFilePermissions0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	keyring.MockInit()
	e := newEnvIn(t, filepath.Join(t.TempDir(), "data")) // created by core.New
	u := e.open(t)                                       // keep open so -wal/-shm exist
	defer u.Close()
	if _, err := u.SetSecret(context.Background(), e.dev.ID, "K", []byte("v"), "", ""); err != nil {
		t.Fatal(err)
	}
	check := func(path string, want os.FileMode, optional bool) {
		t.Helper()
		fi, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) && optional {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", filepath.Base(path), got, want)
		}
	}
	check(e.dbPath, 0o600, false)
	check(e.dbPath+"-wal", 0o600, true)
	check(e.dbPath+"-shm", 0o600, true)
	check(filepath.Join(e.dir, "vaults.json"), 0o600, false)
	check(e.dir, 0o700, false)
	check(filepath.Join(e.dir, "vaults"), 0o700, false)
}

func TestCorruptedBlobDetected(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	u := e.open(t)
	if _, err := u.SetSecret(ctx, e.dev.ID, "K", []byte("v"), "", ""); err != nil {
		t.Fatal(err)
	}
	u.Close()

	db := rawDB(t, e.dbPath)
	var blob []byte
	if err := db.QueryRow(`SELECT blob FROM records WHERE kind='secret'`).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	blob[len(blob)/2] ^= 0xff
	if _, err := db.Exec(`UPDATE records SET blob=? WHERE kind='secret'`, blob); err != nil {
		t.Fatal(err)
	}
	db.Close()

	u = e.open(t)
	defer u.Close()
	if _, err := u.GetSecret(ctx, e.dev.ID, "K"); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("GetSecret = %v, want ErrCorrupt", err)
	}
}

func TestSwapBlobsBetweenRecordsFails(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	u := e.open(t)
	for _, n := range []domain.SecretName{"ONE", "TWO"} {
		if _, err := u.SetSecret(ctx, e.dev.ID, n, []byte("value-"+string(n)), "", ""); err != nil {
			t.Fatal(err)
		}
	}
	u.Close()

	db := rawDB(t, e.dbPath)
	rows, err := db.Query(`SELECT id, blob FROM records WHERE kind='secret' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	var blobs [][]byte
	for rows.Next() {
		var id string
		var b []byte
		if err := rows.Scan(&id, &b); err != nil {
			t.Fatal(err)
		}
		ids, blobs = append(ids, id), append(blobs, b)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatalf("got %d secret records", len(ids))
	}
	for i, id := range ids {
		if _, err := db.Exec(`UPDATE records SET blob=? WHERE kind='secret' AND id=?`, blobs[1-i], id); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	u = e.open(t)
	defer u.Close()
	if _, err := u.ListSecrets(ctx, e.dev.ID); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("ListSecrets = %v, want ErrCorrupt", err)
	}
}

func TestFutureFormatRejected(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	db := rawDB(t, e.dbPath)
	if _, err := db.Exec(`UPDATE meta SET format_version=99`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := e.svc.Unlock(ctx, e.vaultID, []byte(testPw), hour, 0); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatalf("Unlock = %v, want ErrUnsupportedFormat", err)
	}
}

func TestLogsContainNoSecrets(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	ctx := context.Background()
	keyring.MockInit()
	e := newEnvIn(t, t.TempDir())
	u := e.open(t)
	value := []byte("log-probe-value-987654321")
	if _, err := u.SetSecret(ctx, e.dev.ID, "LOG_PROBE", value, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := u.GetSecret(ctx, e.dev.ID, "MISSING"); err == nil { // error path
		t.Fatal("expected not found")
	}
	if _, err := u.ProcessEnv(ctx, e.dev.ID, nil); err != nil {
		t.Fatal(err)
	}
	u.Close()
	wrong := []byte("wrong-password-probe")
	if err := e.svc.Unlock(ctx, e.vaultID, wrong, hour, 0); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	if err := e.svc.Lock(ctx, e.vaultID); err != nil {
		t.Fatal(err)
	}
	for _, p := range [][]byte{value, []byte(testPw), wrong} {
		if bytes.Contains(buf.Bytes(), p) {
			t.Errorf("log contains %q", p)
		}
	}
}
