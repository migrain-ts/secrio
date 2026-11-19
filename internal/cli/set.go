package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// maxValueSize is the largest secret value accepted by `set`.
const maxValueSize = 1 << 20

// openSecretEnv checks that the context names vault, project and env (before
// anything is unlocked), opens the vault and resolves the environment. The
// caller closes the returned vault.
func openSecretEnv(ctx context.Context, d Deps, flags *Flags) (*app.Unlocked, domain.Environment, error) {
	c, err := LoadContext(d, *flags)
	if err != nil {
		return nil, domain.Environment{}, err
	}
	if _, err := c.NeedVault(); err != nil {
		return nil, domain.Environment{}, err
	}
	projectName, err := c.NeedProject()
	if err != nil {
		return nil, domain.Environment{}, err
	}
	envName, err := c.NeedEnv()
	if err != nil {
		return nil, domain.Environment{}, err
	}
	u, err := openVaultFromContext(ctx, d, c)
	if err != nil {
		return nil, domain.Environment{}, err
	}
	p, err := resolveProject(ctx, u, projectName)
	if err == nil {
		var en domain.Environment
		if en, err = resolveEnv(ctx, u, p.ID, envName); err == nil {
			return u, en, nil
		}
	}
	u.Close()
	return nil, domain.Environment{}, err
}

// readSecretValue reads the value for `set`. On a terminal it prompts (masked,
// once); otherwise it reads stdin (max 1 MiB) and drops one trailing line
// terminator. It deliberately ignores SECRIO_PASSWORD. The caller clears the
// result.
func readSecretValue(d Deps) ([]byte, error) {
	if d.StdinTTY() {
		v, err := d.ReadPassword("Value: ")
		if err != nil {
			clear(v)
			return nil, err
		}
		if len(v) == 0 {
			return nil, errors.New("refusing to store an empty value")
		}
		return v, nil
	}
	// One byte past the longest valid input (1 MiB + CRLF), so truncation
	// can never make an oversized input look valid.
	raw, err := io.ReadAll(io.LimitReader(d.In, maxValueSize+3))
	if err != nil {
		clear(raw)
		return nil, fmt.Errorf("reading value from stdin: %w", err)
	}
	v := raw
	if bytes.HasSuffix(v, []byte("\r\n")) {
		v = v[:len(v)-2]
	} else if bytes.HasSuffix(v, []byte("\n")) {
		v = v[:len(v)-1]
	}
	switch {
	case len(raw) > maxValueSize+2 || len(v) > maxValueSize:
		clear(raw)
		return nil, errors.New("value too large (max 1 MiB)")
	case len(v) == 0:
		clear(raw)
		return nil, errors.New("refusing to store an empty value")
	}
	return v, nil
}

func newSetCmd(d Deps, flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "set <NAME>",
		Short: "Set a secret; the value is read from the terminal prompt or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.ParseSecretName(args[0])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			u, en, err := openSecretEnv(ctx, d, flags)
			if err != nil {
				return err
			}
			defer u.Close()
			value, err := readSecretValue(d)
			if err != nil {
				return err
			}
			defer clear(value)
			// A NUL value would make ProcessEnv (and so every `run`) fail for
			// the whole environment.
			if bytes.IndexByte(value, 0) >= 0 {
				return errors.New("refusing to store a value containing NUL bytes (type text)")
			}
			s, err := u.SetSecret(ctx, en.ID, name, value, domain.TypeText, "")
			defer clear(s.Value)
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "set %s\n", name)
			return nil
		},
	}
}
