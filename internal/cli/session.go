package cli

import (
	"context"
	"errors"
	"fmt"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// openVault opens the vault using its session. When that fails for any reason
// (locked, keychain unavailable, damaged session) and SECRIO_PASSWORD is set,
// it opens with the password instead (no session is created). Without the
// variable, a locked vault fails with the "locked" exit code and any other
// error (for example a keychain failure) gets a hint to set SECRIO_PASSWORD.
func openVault(ctx context.Context, d Deps, svc *app.Service, entry app.VaultEntry) (*app.Unlocked, error) {
	u, err := svc.Open(ctx, entry.ID)
	if err == nil {
		return u, nil
	}
	env := d.Getenv("SECRIO_PASSWORD")
	if env == "" {
		if errors.Is(err, domain.ErrLocked) {
			return nil, LockedErr(fmt.Errorf("vault %q is locked: run `secrio unlock`", entry.Name))
		}
		return nil, fmt.Errorf("%w (set SECRIO_PASSWORD to open without the OS keychain)", err)
	}
	pw := []byte(env)
	defer clear(pw)
	u, err = svc.OpenWithPassword(ctx, entry.ID, pw)
	if err != nil {
		return nil, envPasswordErr(d, err)
	}
	return u, nil
}

// selectVault resolves the target vault: the argument if non-empty, else the
// context (flag, env or .secrio.toml).
func selectVault(ctx context.Context, d Deps, svc *app.Service, flags *Flags, argName string) (app.VaultEntry, error) {
	name := argName
	if name == "" {
		c, err := LoadContext(d, *flags)
		if err != nil {
			return app.VaultEntry{}, err
		}
		if name, err = c.NeedVault(); err != nil {
			return app.VaultEntry{}, err
		}
	}
	return resolveVault(ctx, svc, name)
}
