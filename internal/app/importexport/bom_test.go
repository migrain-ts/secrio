package importexport

import (
	"strings"
	"testing"
)

func TestParseStripsBOM(t *testing.T) {
	const bom = "\xEF\xBB\xBF"
	tests := []struct {
		name string
		f    Format
		in   string
	}{
		{"dotenv", FormatDotenv, bom + "A=1\n"},
		{"json", FormatJSON, bom + `{"A":"1"}`},
		{"yaml", FormatYAML, bom + "A: \"1\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.f, strings.NewReader(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if !same(got, entries("A", "1")) {
				t.Fatalf("got %d entries, first name %q", len(got), got[0].Name)
			}
		})
	}
}

func TestDotenvHashShapes(t *testing.T) {
	tests := []struct{ in, want string }{
		{"K=abc #c", "abc"},
		{"K= #c", ""},
		{"K=#abc", "#abc"},
		{"K=a#b", "a#b"},
		{"K=\t#c", ""},
		{"K=", ""},
	}
	for _, tt := range tests {
		got, err := Parse(FormatDotenv, strings.NewReader(tt.in))
		if err != nil || len(got) != 1 || string(got[0].Value) != tt.want {
			t.Errorf("%q: got %v, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestDotenvHashRoundTrip(t *testing.T) {
	for _, v := range []string{"#abc", "a#b", "#", " #c", "x #y"} {
		var buf strings.Builder
		if err := Write(FormatDotenv, &buf, entries("K", v)); err != nil {
			t.Fatal(err)
		}
		got, err := Parse(FormatDotenv, strings.NewReader(buf.String()))
		if err != nil || len(got) != 1 || string(got[0].Value) != v {
			t.Errorf("%q: wrote %q, parsed %v, %v", v, buf.String(), got, err)
		}
	}
}
