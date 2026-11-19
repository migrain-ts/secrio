package importexport

import (
	"fmt"
	"io"

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

func writeYAML(w io.Writer, entries []Entry) error {
	m, err := textMap(entries)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(m) // map[string]string: strings stay strings; keys sorted
	if err != nil {
		return err
	}
	defer clear(b)
	_, err = w.Write(b)
	return err
}
