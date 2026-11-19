package app

import (
	"context"
	"time"

	"secrio/internal/domain"
)

// --- Crypto and time ---

// Cipher is authenticated encryption. Open returns domain.ErrCorrupt for
// truncated input and domain.ErrAuthFailed when authentication fails.
type Cipher interface {
	NewKey() (domain.Key, error)
	Seal(k domain.Key, plaintext, aad []byte) ([]byte, error)
	Open(k domain.Key, sealed, aad []byte) ([]byte, error)
}

type KDF interface {
	RandomSalt() ([]byte, error)
	Derive(password, salt []byte, p domain.KDFParams) domain.Key
}

type Clock interface{ Now() time.Time }

// --- Record storage ---

type Kind string

const (
	KindProject     Kind = "project"
	KindEnvironment Kind = "environment"
	KindSecret      Kind = "secret"
	KindHistory     Kind = "history"
	KindEnvKey      Kind = "envkey"
)

// Record is an opaque sealed blob plus the plaintext routing fields.
type Record struct {
	Kind      Kind
	ID        string
	ParentID  string
	Blob      []byte
	UpdatedAt time.Time
}

// VaultMeta is the plaintext vault header.
type VaultMeta struct {
	FormatVersion   int
	ID              string
	Name            string
	KDF             domain.KDFParams
	Salt            []byte
	WrappedVaultKey []byte
	CreatedAt       time.Time
}

// SessionRow is the single persisted session: an opaque sealed blob plus the
// plaintext timing fields the store needs to enforce expiry atomically.
// Idle == 0 means no idle limit.
type SessionRow struct {
	Blob                []byte
	ExpiresAt, LastUsed time.Time
	Idle                time.Duration
}

type RecordStore interface {
	// PutSession upserts the single session row.
	PutSession(ctx context.Context, r SessionRow) error
	// GetSession returns domain.ErrLocked when there is no session row.
	GetSession(ctx context.Context) (SessionRow, error)
	// TouchSession sets last_used=now in one statement, only if the row exists,
	// is unexpired at now and not idle-expired. It reports whether it did.
	TouchSession(ctx context.Context, now time.Time) (bool, error)
	// DeleteSession removes the session row. It is idempotent.
	DeleteSession(ctx context.Context) error
	Meta(ctx context.Context) (VaultMeta, error)
	PutMeta(ctx context.Context, m VaultMeta) error
	Put(ctx context.Context, r Record) error
	// Get returns domain.ErrNotFound on a miss.
	Get(ctx context.Context, kind Kind, id string) (Record, error)
	List(ctx context.Context, kind Kind, parentID string) ([]Record, error)
	Delete(ctx context.Context, kind Kind, id string) error
	// Tx runs fn in one transaction. fn must use the store it is given.
	Tx(ctx context.Context, fn func(RecordStore) error) error
	// Close releases the store. On the store passed to a Tx callback it is a
	// no-op.
	Close() error
}

// --- Sessions ---

// SessionKeys keeps one random per-session key per vault (the OS keychain in
// production). It never holds the vault key: that lives sealed under the
// session key in the vault file's session row.
type SessionKeys interface {
	Save(ctx context.Context, vaultID string, k domain.Key) error
	// Load returns domain.ErrLocked on a miss.
	Load(ctx context.Context, vaultID string) (domain.Key, error)
	// Delete is idempotent: deleting a missing key is not an error.
	Delete(ctx context.Context, vaultID string) error
}

// --- Vault registry and opening ---

type VaultEntry struct{ ID, Name, Path string }

type Registry interface {
	List(ctx context.Context) ([]VaultEntry, error)
	Add(ctx context.Context, e VaultEntry) error
	Remove(ctx context.Context, id string) error
}

type VaultOpener interface {
	Create(ctx context.Context, path string) (RecordStore, error)
	Open(ctx context.Context, path string) (RecordStore, error)
}
