package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"maps"
	"slices"
	"sort"
	"testing"
	"time"

	"secrio/internal/domain"
)

// testKDF is a cheap deterministic KDF: SHA-256(salt || password).
type testKDF struct{}

func newTestKDF() KDF { return testKDF{} }

func (testKDF) RandomSalt() ([]byte, error) {
	s := make([]byte, 16)
	_, err := rand.Read(s)
	return s, err
}

func (testKDF) Derive(password, salt []byte, _ domain.KDFParams) domain.Key {
	return domain.Key(sha256.Sum256(append(append([]byte{}, salt...), password...)))
}

// testCipher is a stdlib AES-GCM Cipher honouring the port contract:
// truncated input -> ErrCorrupt, authentication failure -> ErrAuthFailed.
type testCipher struct{}

func newTestCipher() Cipher { return testCipher{} }

func gcm(k domain.Key) cipher.AEAD {
	b, err := aes.NewCipher(k[:])
	if err != nil {
		panic(err)
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		panic(err)
	}
	return g
}

func (testCipher) NewKey() (domain.Key, error) {
	var k domain.Key
	_, err := rand.Read(k[:])
	return k, err
}

func (testCipher) Seal(k domain.Key, plaintext, aad []byte) ([]byte, error) {
	g := gcm(k)
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plaintext, aad), nil
}

func (testCipher) Open(k domain.Key, sealed, aad []byte) ([]byte, error) {
	g := gcm(k)
	if len(sealed) < g.NonceSize()+g.Overhead() {
		return nil, domain.ErrCorrupt
	}
	pt, err := g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], aad)
	if err != nil {
		return nil, domain.ErrAuthFailed
	}
	return pt, nil
}

// --- Fake clock ---

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// --- Fake record store (one fakeVault per "file") ---

type recKey struct {
	kind Kind
	id   string
}

type fakeVault struct {
	meta    *VaultMeta
	records map[recKey]Record
	session *SessionRow
	puts    map[Kind]int // Put calls by kind
	handles int          // open, not yet closed outer stores
}

func newFakeVault() *fakeVault {
	return &fakeVault{records: map[recKey]Record{}, puts: map[Kind]int{}}
}

func cloneRecord(r Record) Record { r.Blob = bytes.Clone(r.Blob); return r }

// fakeStore implements RecordStore over a fakeVault. Tx snapshots the vault
// and restores it when fn fails.
type fakeStore struct {
	v      *fakeVault
	tx     bool
	closed bool
}

func (s *fakeStore) check() error {
	if s.closed {
		return errors.New("fake store: closed")
	}
	return nil
}

func (s *fakeStore) Meta(context.Context) (VaultMeta, error) {
	if err := s.check(); err != nil {
		return VaultMeta{}, err
	}
	if s.v.meta == nil {
		return VaultMeta{}, domain.ErrNotFound
	}
	return *s.v.meta, nil
}

func (s *fakeStore) PutMeta(_ context.Context, m VaultMeta) error {
	if err := s.check(); err != nil {
		return err
	}
	s.v.meta = &m
	return nil
}

func (s *fakeStore) Put(_ context.Context, r Record) error {
	if err := s.check(); err != nil {
		return err
	}
	s.v.puts[r.Kind]++
	s.v.records[recKey{r.Kind, r.ID}] = cloneRecord(r)
	return nil
}

func (s *fakeStore) Get(_ context.Context, kind Kind, id string) (Record, error) {
	if err := s.check(); err != nil {
		return Record{}, err
	}
	r, ok := s.v.records[recKey{kind, id}]
	if !ok {
		return Record{}, domain.ErrNotFound
	}
	return cloneRecord(r), nil
}

func (s *fakeStore) List(_ context.Context, kind Kind, parentID string) ([]Record, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	var out []Record
	for k, r := range s.v.records {
		if k.kind == kind && r.ParentID == parentID {
			out = append(out, cloneRecord(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *fakeStore) Delete(_ context.Context, kind Kind, id string) error {
	if err := s.check(); err != nil {
		return err
	}
	delete(s.v.records, recKey{kind, id})
	return nil
}

func (s *fakeStore) Tx(_ context.Context, fn func(RecordStore) error) error {
	if err := s.check(); err != nil {
		return err
	}
	if s.tx {
		return errors.New("fake store: nested transaction")
	}
	snapMeta, snapRecs, snapSess := s.v.meta, maps.Clone(s.v.records), s.v.session
	if err := fn(&fakeStore{v: s.v, tx: true}); err != nil {
		s.v.meta, s.v.records, s.v.session = snapMeta, snapRecs, snapSess
		return err
	}
	return nil
}

func (s *fakeStore) PutSession(_ context.Context, r SessionRow) error {
	if err := s.check(); err != nil {
		return err
	}
	// Normalise like SQLite: times in unix ms (UTC), Idle in whole ms with a
	// positive sub-ms Idle stored as 1 ms.
	r.Blob = bytes.Clone(r.Blob)
	r.ExpiresAt, r.LastUsed = msUTC(r.ExpiresAt), msUTC(r.LastUsed)
	if r.Idle > 0 && r.Idle < time.Millisecond {
		r.Idle = time.Millisecond
	}
	r.Idle = r.Idle.Truncate(time.Millisecond)
	s.v.session = &r
	return nil
}

// msUTC truncates t to unix milliseconds in UTC, as the SQLite store does.
func msUTC(t time.Time) time.Time { return time.UnixMilli(t.UnixMilli()).UTC() }

func (s *fakeStore) GetSession(context.Context) (SessionRow, error) {
	if err := s.check(); err != nil {
		return SessionRow{}, err
	}
	if s.v.session == nil {
		return SessionRow{}, domain.ErrLocked
	}
	r := *s.v.session
	r.Blob = bytes.Clone(r.Blob)
	return r, nil
}

func (s *fakeStore) TouchSession(_ context.Context, now time.Time) (bool, error) {
	if err := s.check(); err != nil {
		return false, err
	}
	r := s.v.session
	if r == nil || now.UnixMilli() >= r.ExpiresAt.UnixMilli() ||
		(r.Idle != 0 && now.UnixMilli()-r.LastUsed.UnixMilli() >= r.Idle.Milliseconds()) {
		return false, nil
	}
	u := *r // copy: Tx snapshot holds the old pointer
	u.LastUsed = msUTC(now)
	s.v.session = &u
	return true, nil
}

func (s *fakeStore) DeleteSession(context.Context) error {
	if err := s.check(); err != nil {
		return err
	}
	s.v.session = nil
	return nil
}

func (s *fakeStore) Close() error {
	if s.tx || s.closed {
		return nil
	}
	s.closed = true
	s.v.handles--
	return nil
}

// countRecords returns how many records of kind exist in the vault.
func (v *fakeVault) countRecords(kind Kind) int {
	n := 0
	for k := range v.records {
		if k.kind == kind {
			n++
		}
	}
	return n
}

// --- Fake opener, registry, session keys ---

type fakeOpener struct{ files map[string]*fakeVault }

func (o *fakeOpener) Create(_ context.Context, path string) (RecordStore, error) {
	if _, ok := o.files[path]; ok {
		return nil, domain.ErrDuplicate
	}
	v := newFakeVault()
	o.files[path] = v
	v.handles++
	return &fakeStore{v: v}, nil
}

func (o *fakeOpener) Open(_ context.Context, path string) (RecordStore, error) {
	v, ok := o.files[path]
	if !ok {
		return nil, domain.ErrNotFound
	}
	v.handles++
	return &fakeStore{v: v}, nil
}

type fakeRegistry struct{ entries []VaultEntry }

func (r *fakeRegistry) List(context.Context) ([]VaultEntry, error) {
	return append([]VaultEntry(nil), r.entries...), nil
}

func (r *fakeRegistry) Add(_ context.Context, e VaultEntry) error {
	r.entries = append(r.entries, e)
	return nil
}

func (r *fakeRegistry) Remove(_ context.Context, id string) error {
	r.entries = slices.DeleteFunc(r.entries, func(e VaultEntry) bool { return e.ID == id })
	return nil
}

// fakeKeys is an in-memory SessionKeys. saved records every key ever saved;
// saveErr and loadErr inject failures.
type fakeKeys struct {
	m       map[string]domain.Key
	saved   []domain.Key
	saveErr error
	loadErr error
}

func (f *fakeKeys) Save(_ context.Context, id string, k domain.Key) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, k)
	f.m[id] = k
	return nil
}

func (f *fakeKeys) Load(_ context.Context, id string) (domain.Key, error) {
	if f.loadErr != nil {
		return domain.Key{}, f.loadErr
	}
	k, ok := f.m[id]
	if !ok {
		return domain.Key{}, domain.ErrLocked
	}
	return k, nil
}

func (f *fakeKeys) Delete(_ context.Context, id string) error {
	delete(f.m, id)
	return nil
}

// --- Harness ---

type harness struct {
	svc    *Service
	clock  *fakeClock
	opener *fakeOpener
	reg    *fakeRegistry
	keys   *fakeKeys
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		clock:  &fakeClock{t: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)},
		opener: &fakeOpener{files: map[string]*fakeVault{}},
		reg:    &fakeRegistry{},
		keys:   &fakeKeys{m: map[string]domain.Key{}},
	}
	h.svc = NewService(newTestCipher(), newTestKDF(), testParams, h.opener, h.reg, h.keys, h.clock)
	t.Cleanup(func() {
		for path, v := range h.opener.files {
			if v.handles != 0 {
				t.Errorf("%s: %d store handle(s) left open", path, v.handles)
			}
		}
	})
	return h
}

const (
	testPath = "/vaults/test.db"
	testPw   = "correct horse battery staple"
)

// newVault creates a vault and returns its id.
func (h *harness) newVault(t *testing.T) string {
	t.Helper()
	v, err := h.svc.CreateVault(context.Background(), "test", testPath, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	return string(v.ID)
}

// unlocked creates a vault, unlocks it and opens a handle closed at cleanup.
func (h *harness) unlocked(t *testing.T) (*Unlocked, string) {
	t.Helper()
	id := h.newVault(t)
	if err := h.svc.Unlock(context.Background(), id, []byte(testPw), time.Hour, 0); err != nil {
		t.Fatal(err)
	}
	u, err := h.svc.Open(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { u.Close() })
	return u, id
}

func (h *harness) vault() *fakeVault { return h.opener.files[testPath] }
