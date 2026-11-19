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

type RecordStore interface {
	Meta(ctx context.Context) (VaultMeta, error)
	PutMeta(ctx context.Context, m VaultMeta) error
	Put(ctx context.Context, r Record) error
	// Get returns domain.ErrNotFound on a miss.
	Get(ctx context.Context, kind Kind, id string) (Record, error)
	List(ctx context.Context, kind Kind, parentID string) ([]Record, error)
	Delete(ctx context.Context, kind Kind, id string) error
	Tx(ctx context.Context, fn func(RecordStore) error) error
}

// --- Sessions ---

type Session struct {
	VaultKey  domain.Key
	ExpiresAt time.Time
	LastUsed  time.Time
	Idle      time.Duration
}

type SessionStore interface {
	Save(ctx context.Context, vaultID string, s Session) error
	// Load returns domain.ErrLocked on a miss.
	Load(ctx context.Context, vaultID string) (Session, error)
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
