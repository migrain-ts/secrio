package app

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"

	"secrio/internal/domain"
)

// envFold: environment variable names are case-insensitive on Windows.
var envFold = runtime.GOOS == "windows"

// envNameEq compares two variable names, case-insensitively when fold.
func envNameEq(a, b string, fold bool) bool {
	if fold {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ProcessEnv returns a NEW slice: base ("KEY=VALUE" entries) with the
// environment's secrets applied. A secret replaces the first base entry of
// the same name in place (later duplicates of that name are dropped); other
// base entries, including malformed ones without '=', are kept in order;
// remaining secrets are appended sorted by name.
//
// The result holds secret values in plaintext: the caller must not log it
// and should drop the reference after exec. A value containing NUL cannot be
// placed in a process environment and yields domain.ErrUnsupportedValue
// (the error names neither the secret nor its value).
func (u *Unlocked) ProcessEnv(ctx context.Context, envID domain.ID[domain.Environment], base []string) ([]string, error) {
	done, lerr := u.enter()
	if lerr != nil {
		return nil, lerr
	}
	defer done()
	secrets, err := u.listSecrets(ctx, envID) // sorted by name
	if err != nil {
		return nil, err
	}
	for _, s := range secrets {
		if bytes.IndexByte(s.Value, 0) >= 0 {
			return nil, fmt.Errorf("%w: secret value cannot be placed in a process environment", domain.ErrUnsupportedValue)
		}
	}
	return mergeEnv(base, secrets, envFold), nil
}

func mergeEnv(base []string, secrets []domain.Secret, fold bool) []string {
	used := make([]bool, len(secrets))
	out := make([]string, 0, len(base)+len(secrets))
	for _, e := range base {
		// Search from index 1 so Windows entries like "=C:=C:\" keep their name.
		i := -1
		if len(e) > 1 {
			if j := strings.IndexByte(e[1:], '='); j >= 0 {
				i = j + 1
			}
		}
		if i < 0 {
			out = append(out, e)
			continue
		}
		k := -1
		for j, s := range secrets {
			if envNameEq(e[:i], string(s.Name), fold) {
				k = j
				break
			}
		}
		switch {
		case k < 0:
			out = append(out, e)
		case !used[k]:
			used[k] = true
			out = append(out, string(secrets[k].Name)+"="+string(secrets[k].Value))
		}
	}
	for k, s := range secrets {
		if !used[k] {
			out = append(out, string(s.Name)+"="+string(s.Value))
		}
	}
	return out
}
