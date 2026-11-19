package app

import (
	"context"

	"secrio/internal/domain"
)

// Presence maps each secret name to whether it exists in each compared
// environment. It carries no values.
type Presence map[domain.SecretName]map[domain.ID[domain.Environment]]bool

// CompareEnvs reports, for every secret name present in at least one of the
// given environments, its presence in every one of them (absent = explicit
// false). Duplicate ids are ignored; an unknown environment is
// domain.ErrNotFound; no ids yields an empty Presence.
func (u *Unlocked) CompareEnvs(ctx context.Context, envIDs ...domain.ID[domain.Environment]) (Presence, error) {
	ids := make([]domain.ID[domain.Environment], 0, len(envIDs))
	seen := map[domain.ID[domain.Environment]]bool{}
	for _, id := range envIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	has := make(map[domain.ID[domain.Environment]]map[domain.SecretName]bool, len(ids))
	names := map[domain.SecretName]bool{}
	for _, id := range ids {
		secrets, err := u.ListSecrets(ctx, id)
		if err != nil {
			return nil, err
		}
		has[id] = make(map[domain.SecretName]bool, len(secrets))
		for _, s := range secrets {
			has[id][s.Name] = true
			names[s.Name] = true
		}
	}
	out := make(Presence, len(names))
	for n := range names {
		row := make(map[domain.ID[domain.Environment]]bool, len(ids))
		for _, id := range ids {
			row[id] = has[id][n]
		}
		out[n] = row
	}
	return out, nil
}
