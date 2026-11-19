package cli

import (
	"context"
	"errors"
	"fmt"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// openVault opens the vault using its session. When the vault is locked and
// SECRIO_PASSWORD is set it opens with the password instead (no session is
// created); otherwise it fails with the "locked" exit code.
func openVault(ctx context.Context, d Deps, svc *app.Service, entry app.VaultEntry) (*app.Unlocked, error) {
	u, err := svc.Open(ctx, entry.ID)
	if err == nil || !errors.Is(err, domain.ErrLocked) {
		return u, err
	}
	env := d.Getenv("SECRIO_PASSWORD")
	if env == "" {
		return nil, LockedErr(fmt.Errorf("vault %q is locked: run `secrio unlock`", entry.Name))
	}
	pw := []byte(env)
	defer clear(pw)
	return svc.OpenWithPassword(ctx, entry.ID, pw)
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
