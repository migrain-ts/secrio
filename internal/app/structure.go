package app

import (
	"context"
	"fmt"
	"sort"

	"secrio/internal/domain"
)

// Record layout:
//   project:     ID=project id, ParentID="",         sealed under the vault key
//   environment: ID=env id,     ParentID=project id, sealed under the vault key
//   envkey:      ID=env id,     ParentID=project id, env key wrapped by the vault key
// Every mutation runs in one Tx closure built from the *Tx helpers below.

// --- Projects ---

func (u *Unlocked) CreateProject(ctx context.Context, name, desc string) (domain.Project, error) {
	if err := validName(name); err != nil {
		return domain.Project{}, err
	}
	p := domain.Project{ID: domain.NewID[domain.Project](), Name: name, Description: desc, CreatedAt: u.clock.Now().UTC()}
	err := u.store.Tx(ctx, func(tx RecordStore) error {
		if err := u.checkProjectNameTx(ctx, tx, name, ""); err != nil {
			return err
		}
		return u.putProjectTx(ctx, tx, p)
	})
	if err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

// ListProjects returns all projects sorted by name.
func (u *Unlocked) ListProjects(ctx context.Context) ([]domain.Project, error) {
	ps, err := listRecords[domain.Project](ctx, u.store, u.c, u.key, KindProject, "")
	if err != nil {
		return nil, err
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
	return ps, nil
}

func (u *Unlocked) RenameProject(ctx context.Context, id domain.ID[domain.Project], newName string) error {
	if err := validName(newName); err != nil {
		return err
	}
	return u.store.Tx(ctx, func(tx RecordStore) error {
		p, err := getRecord[domain.Project](ctx, tx, u.c, u.key, KindProject, string(id))
		if err != nil {
			return err
		}
		if err := u.checkProjectNameTx(ctx, tx, newName, id); err != nil {
			return err
		}
		p.Name = newName
		return u.putProjectTx(ctx, tx, p)
	})
}

// DeleteProject deletes the project with its environments, env keys and
// secrets.
func (u *Unlocked) DeleteProject(ctx context.Context, id domain.ID[domain.Project]) error {
	return u.store.Tx(ctx, func(tx RecordStore) error {
		if _, err := getRecord[domain.Project](ctx, tx, u.c, u.key, KindProject, string(id)); err != nil {
			return err
		}
		envs, err := tx.List(ctx, KindEnvironment, string(id))
		if err != nil {
			return err
		}
		for _, e := range envs {
			if err := u.deleteEnvTx(ctx, tx, domain.ID[domain.Environment](e.ID)); err != nil {
				return err
			}
		}
		return tx.Delete(ctx, KindProject, string(id))
	})
}

// DuplicateProject copies a project, its environments (with NEW env keys)
// and their secrets (re-sealed, new ids) under newName.
func (u *Unlocked) DuplicateProject(ctx context.Context, id domain.ID[domain.Project], newName string) (domain.Project, error) {
	if err := validName(newName); err != nil {
		return domain.Project{}, err
	}
	var cp domain.Project
	err := u.store.Tx(ctx, func(tx RecordStore) error {
		src, err := getRecord[domain.Project](ctx, tx, u.c, u.key, KindProject, string(id))
		if err != nil {
			return err
		}
		if err := u.checkProjectNameTx(ctx, tx, newName, ""); err != nil {
			return err
		}
		cp = domain.Project{ID: domain.NewID[domain.Project](), Name: newName, Description: src.Description, CreatedAt: u.clock.Now().UTC()}
		if err := u.putProjectTx(ctx, tx, cp); err != nil {
			return err
		}
		envs, err := listRecords[domain.Environment](ctx, tx, u.c, u.key, KindEnvironment, string(id))
		if err != nil {
			return err
		}
		for _, e := range envs {
			if err := u.copyEnvTx(ctx, tx, e, cp.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.Project{}, err
	}
	return cp, nil
}

func (u *Unlocked) putProjectTx(ctx context.Context, tx RecordStore, p domain.Project) error {
	return putSealed(ctx, tx, u, u.key, KindProject, string(p.ID), "", p)
}

// checkProjectNameTx returns domain.ErrDuplicate if another project (not
// self) already has name.
func (u *Unlocked) checkProjectNameTx(ctx context.Context, tx RecordStore, name string, self domain.ID[domain.Project]) error {
	ps, err := listRecords[domain.Project](ctx, tx, u.c, u.key, KindProject, "")
	if err != nil {
		return err
	}
	for _, p := range ps {
		if p.Name == name && p.ID != self {
			return fmt.Errorf("project name: %w", domain.ErrDuplicate)
		}
	}
	return nil
}
