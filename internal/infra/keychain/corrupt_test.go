package keychain

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"secrio/internal/domain"
)

// The mock keyring does not decode the go-keyring prefix, so this only covers
// the fall-through to decode; TestGetErrorMapping is the real coverage of
// mapGetErr.
func TestLoadMalformedPrefixedPayloadIsCorrupt(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set("svc", "v", "go-keyring-base64:!!!"); err != nil {
		t.Fatal(err)
	}
	if _, err := New("svc").Load(context.Background(), "v"); !errors.Is(err, domain.ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
}

// go-keyring (macOS) decodes its own prefixed encodings inside Get and
// returns the raw decoder error when that fails.
func TestGetErrorMapping(t *testing.T) {
	_, b64 := base64.StdEncoding.DecodeString("!!!")
	_, hx := hex.DecodeString("zz")
	_, hxLen := hex.DecodeString("abc")
	for name, in := range map[string]error{"base64": b64, "hex": hx, "hexlen": hxLen} {
		err := mapGetErr(in)
		if !errors.Is(err, domain.ErrCorrupt) {
			t.Errorf("%s: want ErrCorrupt, got %v", name, err)
		}
		if strings.Contains(err.Error(), "!!!") {
			t.Errorf("%s: leak: %v", name, err)
		}
	}
	other := errors.New("keychain unavailable")
	if err := mapGetErr(other); errors.Is(err, domain.ErrCorrupt) || !errors.Is(err, other) {
		t.Fatalf("other error mangled: %v", err)
	}
}
