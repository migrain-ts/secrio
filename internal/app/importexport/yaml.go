package importexport

import (
	"fmt"
	"io"
	"unicode"

	"gopkg.in/yaml.v3"

	"secrio/internal/domain"
)

func yamlErr(what string) error {
	return fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, what)
}

// parseYAML decodes into a node tree (keeps order and duplicates). Decoder
// errors are replaced by generic ones.
func parseYAML(data []byte) ([]Entry, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, yamlErr("yaml: malformed document")
	}
	if doc.Kind == 0 { // empty input
		return nil, nil
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, yamlErr("yaml: top level must be a flat map")
	}
	m := doc.Content[0]
	c := newCollector()
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		pos := i/2 + 1
		where := fmt.Sprintf("position %d", pos)
		if k.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%w: unsupported key type at %s", domain.ErrUnsupportedValue, where)
		}
		if _, err := domain.ParseSecretName(k.Value); err != nil {
			return nil, fmt.Errorf("%w: invalid name at %s", domain.ErrInvalidName, where)
		}
		if v.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%w: unsupported value type for key at %s", domain.ErrUnsupportedValue, where)
		}
		switch v.ShortTag() {
		case "!!str", "!!int", "!!float", "!!bool":
		default: // !!null and anything exotic
			return nil, fmt.Errorf("%w: unsupported value type for key at %s", domain.ErrUnsupportedValue, where)
		}
		if err := c.add(where, k.Value, []byte(v.Value)); err != nil {
			return nil, err
		}
	}
	return c.out, nil
}

// yamlNeedsDoubleQuotes reports values the plain/block styles can corrupt:
// control characters, line breaks and the Unicode line separators.
func yamlNeedsDoubleQuotes(v string) bool {
	for _, r := range v {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return true
		}
	}
	return false
}

func writeYAML(w io.Writer, entries []Entry) error {
	m, err := textMap(entries) // validates UTF-8
	if err != nil {
		return err
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, e := range entries { // already sorted by Write
		v := m[string(e.Name)]
		val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
		if yamlNeedsDoubleQuotes(v) {
			val.Style = yaml.DoubleQuotedStyle
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(e.Name)}, val)
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	defer clear(b)
	_, err = w.Write(b)
	return err
}
