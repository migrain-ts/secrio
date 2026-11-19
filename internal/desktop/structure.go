package desktop

import (
	"context"
	"sort"
	"strings"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// envDTOs lists a project's environments with their secret counts (no
// decryption of secrets), sorted by name case-insensitively.
func envDTOs(ctx context.Context, u *app.Unlocked, pid domain.ID[domain.Project]) ([]EnvDTO, error) {
	envs, err := u.ListEnvironments(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]EnvDTO, len(envs))
	for i, e := range envs {
		n, err := u.SecretCount(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		out[i] = EnvDTO{ID: string(e.ID), Name: e.Name, SecretCount: n}
	}
	sort.SliceStable(out, func(i, j int) bool { return lessName(out[i].Name, out[j].Name) })
	return out, nil
}

func lessName(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

// ListProjects returns the projects sorted by name with their totals.
func (a *App) ListProjects() ([]ProjectDTO, error) {
	var out []ProjectDTO
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		ps, err := u.ListProjects(ctx)
		if err != nil {
			return err
		}
		out = make([]ProjectDTO, len(ps))
		for i, p := range ps {
			es, err := envDTOs(ctx, u, p.ID)
			if err != nil {
				return err
			}
			dto := ProjectDTO{ID: string(p.ID), Name: p.Name, EnvCount: len(es)}
			for _, e := range es {
				dto.SecretCount += e.SecretCount
			}
			out[i] = dto
		}
		sort.SliceStable(out, func(i, j int) bool { return lessName(out[i].Name, out[j].Name) })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateProject creates a project; the name is passed to the Core untrimmed.
func (a *App) CreateProject(name string) (ProjectDTO, error) {
	var out ProjectDTO
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		p, err := u.CreateProject(ctx, name, "")
		if err != nil {
			return err
		}
		out = ProjectDTO{ID: string(p.ID), Name: p.Name}
		return nil
	})
	if err != nil {
		return ProjectDTO{}, err
	}
	return out, nil
}

// DeleteProject deletes a project with its environments and secrets.
func (a *App) DeleteProject(id string) error {
	return a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		return u.DeleteProject(ctx, domain.ID[domain.Project](id))
	})
}

// ListEnvironments returns the project's environments sorted by name.
func (a *App) ListEnvironments(projectID string) ([]EnvDTO, error) {
	var out []EnvDTO
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		var err error
		out, err = envDTOs(ctx, u, domain.ID[domain.Project](projectID))
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateEnvironment creates an environment in the project.
func (a *App) CreateEnvironment(projectID, name string) (EnvDTO, error) {
	var out EnvDTO
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		e, err := u.CreateEnvironment(ctx, domain.ID[domain.Project](projectID), name)
		if err != nil {
			return err
		}
		out = EnvDTO{ID: string(e.ID), Name: e.Name}
		return nil
	})
	if err != nil {
		return EnvDTO{}, err
	}
	return out, nil
}

// DeleteEnvironment deletes an environment with its secrets.
func (a *App) DeleteEnvironment(id string) error {
	return a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		return u.DeleteEnvironment(ctx, domain.ID[domain.Environment](id))
	})
}
