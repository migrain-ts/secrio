package cli

import (
	"context"
	"fmt"
	"regexp"

	"secrio/internal/app"
	"secrio/internal/domain"
)

var vaultNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// validVaultName checks a vault name against the CLI naming rule.
func validVaultName(name string) error {
	// "." and ".." match the pattern but are path components, not names.
	if !vaultNameRE.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid vault name %q: use letters, digits, '.', '_' or '-' (max 64)", name)
	}
	return nil
}

// resolveVault finds a registered vault by exact name; a duplicate name is an
// error rather than a silent first match.
func resolveVault(ctx context.Context, svc *app.Service, name string) (app.VaultEntry, error) {
	entries, err := svc.ListVaults(ctx)
	if err != nil {
		return app.VaultEntry{}, err
	}
	var found []app.VaultEntry
	for _, e := range entries {
		if e.Name == name {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 0:
		return app.VaultEntry{}, fmt.Errorf("unknown vault %q", name)
	case 1:
		return found[0], nil
	default:
		return app.VaultEntry{}, fmt.Errorf("ambiguous vault name %q", name)
	}
}

func resolveProject(ctx context.Context, u *app.Unlocked, name string) (domain.Project, error) {
	ps, err := u.ListProjects(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	for _, p := range ps {
		if p.Name == name {
			return p, nil
		}
	}
	return domain.Project{}, fmt.Errorf("unknown project %q", name)
}

func resolveEnv(ctx context.Context, u *app.Unlocked, projectID domain.ID[domain.Project], name string) (domain.Environment, error) {
	es, err := u.ListEnvironments(ctx, projectID)
	if err != nil {
		return domain.Environment{}, err
	}
	for _, e := range es {
		if e.Name == name {
			return e, nil
		}
	}
	return domain.Environment{}, fmt.Errorf("unknown environment %q", name)
}
