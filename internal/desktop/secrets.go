package desktop

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"secrio/internal/app"
	"secrio/internal/domain"
)

// maxSecretValue is the longest value accepted, same as the CLI.
const maxSecretValue = 1 << 20

var secretTypes = map[domain.SecretType]bool{
	domain.TypeText: true, domain.TypePassword: true, domain.TypeAPIKey: true, domain.TypeToken: true,
	domain.TypeURL: true, domain.TypePrivateKey: true, domain.TypeCertificate: true, domain.TypeJSON: true,
}

func eid(s string) domain.ID[domain.Environment] { return domain.ID[domain.Environment](s) }

// ListSecrets returns the environment's secrets (metadata only, never values)
// sorted case-sensitively by name. Values are cleared right after listing.
func (a *App) ListSecrets(envID string) ([]SecretDTO, error) {
	var out []SecretDTO
	err := a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		ss, err := u.ListSecrets(ctx, eid(envID))
		if err != nil {
			return err
		}
		out = make([]SecretDTO, 0, len(ss))
		for i := range ss {
			s := ss[i]
			clear(s.Value)
			out = append(out, SecretDTO{
				Name:        string(s.Name),
				Type:        string(s.Type),
				Description: s.Description,
				CreatedAt:   s.CreatedAt.UTC().Format(time.RFC3339),
				UpdatedAt:   s.UpdatedAt.UTC().Format(time.RFC3339),
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SaveSecret creates or updates a secret. The UI sends the value here; it is
// never returned. An empty typ keeps the existing type, else uses text; an
// update keeps the existing description.
func (a *App) SaveSecret(envID, name, value, typ string) error {
	n, err := domain.ParseSecretName(name)
	if err != nil {
		return newAPIError(CodeInvalidName, "invalid name")
	}
	if value == "" || len(value) > maxSecretValue || strings.IndexByte(value, 0) >= 0 {
		return newAPIError(CodeInvalidValue, "invalid value")
	}
	st := domain.SecretType(typ)
	if st != "" && !secretTypes[st] {
		return newAPIError(CodeInvalidValue, "invalid value")
	}
	return a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		desc := ""
		old, err := u.GetSecret(ctx, eid(envID), n)
		switch {
		case err == nil:
			clear(old.Value)
			desc = old.Description
			if st == "" {
				st = old.Type
			}
		case errors.Is(err, domain.ErrNotFound):
			// create (an unknown environment fails again in SetSecret)
		default:
			return err
		}
		b := []byte(value)
		defer clear(b)
		s, err := u.SetSecret(ctx, eid(envID), n, b, st, desc)
		clear(s.Value)
		return err
	})
}

// DeleteSecret removes a secret.
func (a *App) DeleteSecret(envID, name string) error {
	n, err := domain.ParseSecretName(name)
	if err != nil {
		return newAPIError(CodeInvalidName, "invalid name")
	}
	return a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		return u.DeleteSecret(ctx, eid(envID), n)
	})
}

// RevealSecret is the ONLY method that returns a secret value. The Core's
// byte slice is cleared, but the returned Go string is an unavoidable copy
// that cannot be zeroed.
func (a *App) RevealSecret(envID, name string) (string, error) {
	n, err := domain.ParseSecretName(name)
	if err != nil {
		return "", newAPIError(CodeInvalidName, "invalid name")
	}
	var out string
	err = a.withVault(func(ctx context.Context, _ *app.Service, _ app.VaultEntry, u *app.Unlocked) error {
		s, err := u.GetSecret(ctx, eid(envID), n)
		if err != nil {
			return err
		}
		out = string(s.Value)
		clear(s.Value)
		return nil
	})
	if err != nil {
		return "", err
	}
	return out, nil
}
