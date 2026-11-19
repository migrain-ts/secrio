package importexport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"secrio/internal/domain"
)

func jsonErr(what string) error {
	return fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, what)
}

// parseJSON reads a flat object token by token so key order and duplicates
// are preserved. Decoder errors are replaced by generic ones: they can quote
// input bytes.
func parseJSON(data []byte) ([]Entry, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, jsonErr("json: top level must be an object")
	}
	c := newCollector()
	for pos := 1; dec.More(); pos++ {
		kt, err := dec.Token()
		key, ok := kt.(string)
		if err != nil || !ok {
			return nil, jsonErr("json: malformed object")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, jsonErr("json: malformed value")
		}
		val, ok := jsonScalar(raw)
		where := fmt.Sprintf("position %d", pos)
		if _, nerr := domain.ParseSecretName(key); nerr != nil {
			return nil, fmt.Errorf("%w: invalid name at %s", domain.ErrInvalidName, where)
		}
		if !ok {
			return nil, fmt.Errorf("%w: unsupported value type for key at %s", domain.ErrUnsupportedValue, where)
		}
		if err := c.add(where, key, val); err != nil {
			return nil, err
		}
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return nil, jsonErr("json: malformed object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, jsonErr("json: trailing data")
	}
	return c.out, nil
}

// jsonScalar converts string, number and bool; null, arrays and objects fail.
func jsonScalar(raw json.RawMessage) ([]byte, bool) {
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, false
		}
		return []byte(s), true
	case 't', 'f', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return bytes.Clone(raw), true
	}
	return nil, false
}

func writeJSON(w io.Writer, entries []Entry) error {
	m, err := textMap(entries)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ") // map keys are sorted
	if err != nil {
		return err
	}
	defer clear(b)
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
