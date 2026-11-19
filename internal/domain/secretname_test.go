package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseSecretName(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"DATABASE_URL", true}, {"_x", true}, {"a1", true},
		{"", false}, {"1A", false}, {"A-B", false}, {"A B", false}, {"é", false},
	}
	for _, c := range cases {
		got, err := ParseSecretName(c.in)
		if c.valid {
			if err != nil || string(got) != c.in {
				t.Errorf("%q: got %q, %v", c.in, got, err)
			}
		} else if !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q: want ErrInvalidName, got %v", c.in, err)
		}
	}
}

func TestParseSecretNameCaseSensitive(t *testing.T) {
	a, _ := ParseSecretName("a")
	b, _ := ParseSecretName("A")
	if a == b || a != "a" || b != "A" {
		t.Fatalf("case not preserved: %q %q", a, b)
	}
}

func TestSecretFormatRedacted(t *testing.T) {
	s := Secret{Value: []byte("topsecret")}
	out := fmt.Sprintf("%v %+v %#v %s", s, s, s, &s)
	if strings.Contains(out, "topsecret") || strings.Contains(out, "116") {
		t.Fatalf("leaked: %s", out)
	}
	if !strings.Contains(out, "Secret{<redacted>}") {
		t.Fatalf("not redacted: %s", out)
	}
}

func TestSecretFormatRedactsAllVerbs(t *testing.T) {
	val := []byte("supersecret")
	s := Secret{Value: val}
	out := fmt.Sprintf("%d %x %+v %v %#v %s %X %q", s, s, s, s, s, s, s, s)
	dec := strings.Trim(fmt.Sprint(val), "[]") // "115 117 112 ..."
	hexs := fmt.Sprintf("%x", val)             // "7375706572..."
	for _, bad := range []string{"supersecret", dec, "115 117", hexs, "73757065"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
			t.Fatalf("leaked %q in %s", bad, out)
		}
	}
	if strings.Count(out, "Secret{<redacted>}") != 8 {
		t.Fatalf("not redacted: %s", out)
	}
}

func TestKeyFormatRedacted(t *testing.T) {
	var k Key
	k[0] = 0xAB
	out := fmt.Sprintf("%v %+v %#v %s %x", k, k, k, &k, k)
	if strings.Contains(out, "171") || strings.Contains(strings.ToLower(out), "ab00") {
		t.Fatalf("leaked: %s", out)
	}
	if !strings.Contains(out, "Key{<redacted>}") {
		t.Fatalf("not redacted: %s", out)
	}
}
