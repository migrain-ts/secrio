// Package importexport parses and writes secret sets as dotenv, JSON or YAML.
// It depends only on domain. Error messages never contain secret names or
// values; they report line numbers (dotenv) or entry positions (json/yaml).
package importexport

import (
	"fmt"
	"io"
	"sort"
	"unicode/utf8"

	"secrio/internal/domain"
)

type Format string

const (
	FormatDotenv Format = "dotenv"
	FormatJSON   Format = "json"
	FormatYAML   Format = "yaml"
)

// MaxInputBytes caps what Parse reads (8 MiB).
const MaxInputBytes = 8 << 20

// Entry is one name/value pair.
type Entry struct {
	Name  domain.SecretName
	Value []byte
}

const redactedEntry = "Entry{<redacted>}"

func (Entry) String() string   { return redactedEntry }
func (Entry) GoString() string { return redactedEntry }

// Format redacts the entry for every fmt verb.
func (Entry) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redactedEntry)) }

// Parse reads entries from r. Duplicate names: the last value wins, at the
// position of the first appearance.
func Parse(f Format, r io.Reader) ([]Entry, error) {
	if f != FormatDotenv && f != FormatJSON && f != FormatYAML {
		return nil, fmt.Errorf("%w: unknown format", domain.ErrUnsupportedFormat)
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	defer clear(data)
	if len(data) > MaxInputBytes {
		return nil, fmt.Errorf("%w: input exceeds 8 MiB", domain.ErrUnsupportedValue)
	}
	switch f {
	case FormatDotenv:
		return parseDotenv(data)
	case FormatJSON:
		return parseJSON(data)
	default:
		return parseYAML(data)
	}
}

// Write writes entries sorted by name; the caller's slice is not reordered.
func Write(f Format, w io.Writer, entries []Entry) error {
	sorted := append([]Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	switch f {
	case FormatDotenv:
		return writeDotenv(w, sorted)
	case FormatJSON:
		return writeJSON(w, sorted)
	case FormatYAML:
		return writeYAML(w, sorted)
	}
	return fmt.Errorf("%w: unknown format", domain.ErrUnsupportedFormat)
}

// textMap converts entries for the structured writers, which only carry
// valid UTF-8 text.
func textMap(entries []Entry) (map[string]string, error) {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		if !utf8.Valid(e.Value) {
			return nil, fmt.Errorf("%w: value is not valid UTF-8 text", domain.ErrUnsupportedValue)
		}
		m[string(e.Name)] = string(e.Value)
	}
	return m, nil
}

// collector accumulates entries with last-wins duplicate handling.
type collector struct {
	out []Entry
	idx map[domain.SecretName]int
}

func newCollector() *collector { return &collector{idx: map[domain.SecretName]int{}} }

// add validates name; where identifies the entry in errors ("line 3", "position 2").
func (c *collector) add(where, name string, value []byte) error {
	n, err := domain.ParseSecretName(name)
	if err != nil {
		return fmt.Errorf("%w: invalid name at %s", domain.ErrInvalidName, where)
	}
	if i, ok := c.idx[n]; ok {
		c.out[i].Value = value
		return nil
	}
	c.idx[n] = len(c.out)
	c.out = append(c.out, Entry{Name: n, Value: value})
	return nil
}
