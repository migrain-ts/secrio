package importexport

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"

	"secrio/internal/domain"
)

func dotenvErr(line int, what string) error {
	return fmt.Errorf("%w: %s at line %d", domain.ErrUnsupportedFormat, what, line)
}

func parseDotenv(data []byte) ([]Entry, error) {
	c := newCollector()
	for i, raw := range strings.Split(string(data), "\n") {
		n := i + 1
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || line[0] == '#' {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, dotenvErr(n, "missing '='")
		}
		v, err := dotenvValue(strings.TrimLeft(val, " \t"), n)
		if err != nil {
			return nil, err
		}
		if err := c.add(fmt.Sprintf("line %d", n), strings.TrimSpace(key), []byte(v)); err != nil {
			return nil, err
		}
	}
	return c.out, nil
}

// dotenvValue decodes one value (already left-trimmed, no trailing newline).
func dotenvValue(s string, line int) (string, error) {
	if s == "" {
		return "", nil
	}
	switch s[0] {
	case '\'':
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", dotenvErr(line, "unterminated quote")
		}
		return s[1 : 1+end], trailingOK(s[end+2:], line)
	case '"':
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			switch ch := s[i]; ch {
			case '"':
				return b.String(), trailingOK(s[i+1:], line)
			case '\\':
				if i+1 >= len(s) {
					return "", dotenvErr(line, "unterminated quote")
				}
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '"', '\\':
					b.WriteByte(s[i])
				default: // unknown escape stays literal
					b.WriteByte('\\')
					b.WriteByte(s[i])
				}
			default:
				b.WriteByte(ch)
			}
		}
		return "", dotenvErr(line, "unterminated quote")
	}
	if s[0] == '#' {
		return "", nil
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s), nil
}

// trailingOK accepts only blanks or a comment after a closing quote.
func trailingOK(rest string, line int) error {
	rest = strings.TrimSpace(rest)
	if rest == "" || rest[0] == '#' {
		return nil
	}
	return dotenvErr(line, "unexpected text after quoted value")
}

func dotenvNeedsQuote(v []byte) bool {
	for _, r := range string(v) {
		if unicode.IsSpace(r) || strings.ContainsRune("#\"'\\$", r) {
			return true
		}
	}
	return false
}

var dotenvEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)

func writeDotenv(w io.Writer, entries []Entry) error {
	var buf bytes.Buffer
	defer clear(buf.Bytes())
	for _, e := range entries {
		buf.WriteString(string(e.Name))
		buf.WriteByte('=')
		if dotenvNeedsQuote(e.Value) {
			buf.WriteByte('"')
			dotenvEscaper.WriteString(&buf, string(e.Value))
			buf.WriteByte('"')
		} else {
			buf.Write(e.Value)
		}
		buf.WriteByte('\n')
	}
	_, err := w.Write(buf.Bytes())
	return err
}
