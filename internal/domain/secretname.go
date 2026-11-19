package domain

import "fmt"

// SecretName is a case-sensitive identifier matching [A-Za-z_][A-Za-z0-9_]*.
type SecretName string

// ParseSecretName validates s and returns it unchanged as a SecretName.
func ParseSecretName(s string) (SecretName, error) {
	if s == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidName)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !letter && !(i > 0 && c >= '0' && c <= '9') {
			return "", fmt.Errorf("%w: must match [A-Za-z_][A-Za-z0-9_]*", ErrInvalidName)
		}
	}
	return SecretName(s), nil
}
