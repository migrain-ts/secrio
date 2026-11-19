package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"secrio/internal/app"
	"secrio/internal/domain"
)

var vaultNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// validVaultName checks a vault name against the CLI naming rule.
func validVaultName(name string) error {
	// Names that match the pattern but are path components (".", ".."), hidden
	// or contain "..", or are Windows device names (also with an extension).
	if !vaultNameRE.MatchString(name) || strings.HasPrefix(name, ".") ||
		strings.Contains(name, "..") || windowsReserved(name) {
		return fmt.Errorf("invalid vault name %q: use letters, digits, '.', '_' or '-' (max 64)", name)
	}
	return nil
}

// windowsReserved reports whether name (case-insensitively, ignoring any
// extension) is a Windows device name: con prn aux nul com1-9 lpt1-9.
func windowsReserved(name string) bool {
	base, _, _ := strings.Cut(strings.ToLower(name), ".")
	switch base {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(base) == 4 && (base[:3] == "com" || base[:3] == "lpt") && base[3] >= '1' && base[3] <= '9'
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
