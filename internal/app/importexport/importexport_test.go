package importexport

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"secrio/internal/domain"
)

func entries(kv ...string) []Entry {
	var out []Entry
	for i := 0; i < len(kv); i += 2 {
		out = append(out, Entry{Name: domain.SecretName(kv[i]), Value: []byte(kv[i+1])})
	}
	return out
}

func same(a, b []Entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !bytes.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func TestParseDotenvTable(t *testing.T) {
	tests := []struct {
		name, in string
		want     []Entry
	}{
		{"basic", "A=1\nB=two", entries("A", "1", "B", "two")},
		{"export", "export A=1\nexport  B=2\n", entries("A", "1", "B", "2")},
		{"comments and blanks", "# c\n\nA=1\n  # indented\n", entries("A", "1")},
		{"equals in value", "A=x=y=z", entries("A", "x=y=z")},
		{"empty value", "A=\nB=''\nC=\"\"", entries("A", "", "B", "", "C", "")},
		{"crlf", "A=1\r\nB=\"x\"\r\n", entries("A", "1", "B", "x")},
		{"single quoted literal", `A='a\nb $x # y'`, entries("A", `a\nb $x # y`)},
		{"double escapes", `A="a\nb\r\t\"\\"`, entries("A", "a\nb\r\t\"\\")},
		{"unquoted trimmed", "A=  hi there  ", entries("A", "hi there")},
		{"inline comment unquoted", "A=val # note", entries("A", "val")},
		{"hash without space kept", "A=va#l", entries("A", "va#l")},
		{"inline comment after quoted", `A="v # x" # note`, entries("A", "v # x")},
		{"hash inside quotes kept", `A='v # x'`, entries("A", "v # x")},
		{"key whitespace", "  A  =1", entries("A", "1")},
		{"duplicate last wins first position", "A=1\nB=2\nA=3", entries("A", "3", "B", "2")},
		{"unicode", "A=héllo✓", entries("A", "héllo✓")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(FormatDotenv, strings.NewReader(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if !same(got, tt.want) {
				t.Fatalf("got %d entries, mismatch", len(got))
			}
		})
	}
}

func TestParseInvalidNameReportsLineNotValue(t *testing.T) {
	const secret = "hunter2-VALUE"
	tests := []struct {
		f  Format
		in string
	}{
		{FormatDotenv, "OK=1\n\n1BAD=" + secret},
		{FormatDotenv, "OK=1\nba-d=" + secret},
		{FormatJSON, `{"OK":"1","ba-d":"` + secret + `"}`},
		{FormatYAML, "OK: 1\nba-d: " + secret + "\n"},
	}
	for _, tt := range tests {
		_, err := Parse(tt.f, strings.NewReader(tt.in))
		if !errors.Is(err, domain.ErrInvalidName) {
			t.Fatalf("%s: want ErrInvalidName, got %v", tt.f, err)
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "BAD") || strings.Contains(err.Error(), "ba-d") {
			t.Fatalf("%s: error leaks content: %v", tt.f, err)
		}
	}
	_, err := Parse(FormatDotenv, strings.NewReader("OK=1\n\n1BAD=x"))
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("want line 3, got %v", err)
	}
	_, err = Parse(FormatJSON, strings.NewReader(`{"OK":"1","ba-d":"x"}`))
	if !strings.Contains(err.Error(), "position 2") {
		t.Fatalf("want position 2, got %v", err)
	}
}

func TestParseDotenvSyntaxErrorsNoContent(t *testing.T) {
	for _, in := range []string{"A=\"unterminated SECRETVAL", "A='unterminated SECRETVAL", "A=\"a\nb\"", "NOEQUALSSECRETVAL", `A="x" trailingSECRETVAL`} {
		_, err := Parse(FormatDotenv, strings.NewReader(in))
		if err == nil {
			t.Fatalf("%q: want error", in)
		}
		if strings.Contains(err.Error(), "SECRETVAL") || !strings.Contains(err.Error(), "line") {
			t.Fatalf("%q: bad error %v", in, err)
		}
	}
}

func TestParseJSONYAMLFlatMaps(t *testing.T) {
	want := entries("A", "1", "B", "two words")
	got, err := Parse(FormatJSON, strings.NewReader(`{"A":"1","B":"two words"}`))
	if err != nil || !same(got, want) {
		t.Fatalf("json: %v", err)
	}
	got, err = Parse(FormatYAML, strings.NewReader("A: \"1\"\nB: two words\n"))
	if err != nil || !same(got, want) {
		t.Fatalf("yaml: %v", err)
	}
}

func TestParseScalars(t *testing.T) {
	want := entries("S", "x", "N", "12.50", "I", "7", "T", "true", "F", "false")
	got, err := Parse(FormatJSON, strings.NewReader(`{"S":"x","N":12.50,"I":7,"T":true,"F":false}`))
	if err != nil || !same(got, want) {
		t.Fatalf("json: %v", err)
	}
	got, err = Parse(FormatYAML, strings.NewReader("S: x\nN: 12.50\nI: 7\nT: true\nF: false\n"))
	if err != nil || !same(got, want) {
		t.Fatalf("yaml: %v", err)
	}
}

func TestParseRejectsContainersAndNull(t *testing.T) {
	const secret = "SECRETNAME"
	ins := map[Format][]string{
		FormatJSON: {`{"` + secret + `":null}`, `{"` + secret + `":[1]}`, `{"` + secret + `":{"a":"b"}}`, `["a"]`, `"x"`, `{"A":"1"} {}`, `{`},
		FormatYAML: {secret + ": null\n", secret + ": ~\n", secret + ": [1]\n", secret + ": {a: b}\n", "- a\n", "just a string\n", secret + ":\n"},
	}
	for f, list := range ins {
		for _, in := range list {
			_, err := Parse(f, strings.NewReader(in))
			if err == nil {
				t.Fatalf("%s %q: want error", f, in)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("%s %q: leaks name: %v", f, in, err)
			}
		}
	}
	_, err := Parse(FormatJSON, strings.NewReader(`{"A":"1","B":null}`))
	if err == nil || !strings.Contains(err.Error(), "position 2") {
		t.Fatalf("want position 2, got %v", err)
	}
}

func TestParseDuplicateLastWins(t *testing.T) {
	want := entries("A", "3", "B", "2")
	got, err := Parse(FormatJSON, strings.NewReader(`{"A":"1","B":"2","A":"3"}`))
	if err != nil || !same(got, want) {
		t.Fatalf("json: %v", err)
	}
	got, err = Parse(FormatYAML, strings.NewReader("A: 1\nB: 2\nA: 3\n"))
	if err != nil || !same(got, want) {
		t.Fatalf("yaml: %v", err)
	}
}

func TestParseSizeCap(t *testing.T) {
	for _, f := range []Format{FormatDotenv, FormatJSON, FormatYAML} {
		_, err := Parse(f, strings.NewReader(strings.Repeat("a", MaxInputBytes+1)))
		if !errors.Is(err, domain.ErrUnsupportedValue) {
			t.Fatalf("%s: want ErrUnsupportedValue, got %v", f, err)
		}
	}
	if _, err := Parse(FormatDotenv, strings.NewReader("A="+strings.Repeat("a", MaxInputBytes-2))); err != nil {
		t.Fatalf("exactly at cap: %v", err)
	}
}

func TestUnknownFormat(t *testing.T) {
	if _, err := Parse("xml", strings.NewReader("")); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatal(err)
	}
	if err := Write("xml", &bytes.Buffer{}, nil); !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Fatal(err)
	}
}

func TestRoundTripAllFormats(t *testing.T) {
	in := entries(
		"A_NEWLINE", "line1\nline2\r\nline3",
		"B_QUOTES", `he said "hi" and 'bye'`,
		"C_EQUALS", "k=v=w",
		"D_HASH", "a # b #c",
		"E_SPACES", "  lead and trail  ",
		"F_UNICODE", "héllo ✓ 日本 \u00a0x",
		"G_BACKSLASH", `C:\path\n \"`,
		"H_DOLLAR", "$HOME ${X} $$",
		"I_EMPTY", "",
		"J_NUMERIC", "123",
		"K_BOOL", "true",
		"L_TAB", "a\tb",
		"M_PLAIN", "plain",
		"N_COLON", "a: b",
		"O_HASHONLY", "#start",
		"P_NBSP", "\u00a0x",
	)
	for _, f := range []Format{FormatDotenv, FormatJSON, FormatYAML} {
		t.Run(string(f), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(f, &buf, in); err != nil {
				t.Fatal(err)
			}
			got, err := Parse(f, &buf)
			if err != nil {
				t.Fatal(err)
			}
			if !same(got, in) { // in is already sorted by name
				for i := range got {
					if i < len(in) && !bytes.Equal(got[i].Value, in[i].Value) {
						t.Errorf("%s: value mismatch", in[i].Name)
					}
				}
				t.Fatalf("round trip mismatch (%d vs %d entries)", len(got), len(in))
			}
		})
	}
}

func TestWriteSortedAndShapes(t *testing.T) {
	in := entries("B", "2", "A", "x y", "C", "")
	var buf bytes.Buffer
	if err := Write(FormatDotenv, &buf, in); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "A=\"x y\"\nB=2\nC=\n" {
		t.Fatalf("dotenv: %q", buf.String())
	}
	buf.Reset()
	if err := Write(FormatJSON, &buf, in); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "{\n  \"A\"") {
		t.Fatalf("json: %q", buf.String())
	}
	if in[0].Name != "B" {
		t.Fatal("Write must not reorder the caller's slice")
	}
}

func TestWriteRejectsInvalidUTF8ForJSONYAML(t *testing.T) {
	in := []Entry{{Name: "A", Value: []byte{0xff, 0xfe}}}
	for _, f := range []Format{FormatJSON, FormatYAML} {
		err := Write(f, &bytes.Buffer{}, in)
		if !errors.Is(err, domain.ErrUnsupportedValue) {
			t.Fatalf("%s: %v", f, err)
		}
	}
}

func TestEntryRedacted(t *testing.T) {
	e := Entry{Name: "A", Value: []byte("topsecret")}
	for _, s := range []string{fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e), fmt.Sprintf("%s", e)} {
		if strings.Contains(s, "topsecret") {
			t.Fatal(s)
		}
	}
}
