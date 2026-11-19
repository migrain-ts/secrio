package app

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"secrio/internal/app/importexport"
	"secrio/internal/domain"
)

type Strategy string

const (
	StrategySkip      Strategy = "skip"
	StrategyOverwrite Strategy = "overwrite"
)

// ImportPlan classifies parsed entries against an environment. The values
// stay in the unexported entries and are redacted under every fmt verb.
type ImportPlan struct {
	New, Duplicate []domain.SecretName
	entries        []importexport.Entry
}

const redactedPlan = "ImportPlan{<redacted>}"

func (ImportPlan) String() string   { return redactedPlan }
func (ImportPlan) GoString() string { return redactedPlan }

// Format redacts the plan for every fmt verb.
func (ImportPlan) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redactedPlan)) }

// PlanImport lists which entries are new and which already exist in envID.
func (u *Unlocked) PlanImport(ctx context.Context, envID domain.ID[domain.Environment], entries []importexport.Entry) (ImportPlan, error) {
	existing, err := u.ListSecrets(ctx, envID)
	if err != nil {
		return ImportPlan{}, err
	}
	have := map[domain.SecretName]bool{}
	for _, s := range existing {
		have[s.Name] = true
	}
	seen := map[domain.SecretName]bool{}
	plan := ImportPlan{entries: make([]importexport.Entry, 0, len(entries))}
	for i, e := range entries {
		if _, err := domain.ParseSecretName(string(e.Name)); err != nil {
			return ImportPlan{}, fmt.Errorf("%w: invalid name at position %d", domain.ErrInvalidName, i+1)
		}
		if seen[e.Name] {
			return ImportPlan{}, fmt.Errorf("%w: repeated name at position %d", domain.ErrDuplicate, i+1)
		}
		seen[e.Name] = true
		if have[e.Name] {
			plan.Duplicate = append(plan.Duplicate, e.Name)
		} else {
			plan.New = append(plan.New, e.Name)
		}
		plan.entries = append(plan.entries, importexport.Entry{Name: e.Name, Value: bytes.Clone(e.Value)})
	}
	return plan, nil
}

// ApplyImport stores the plan's entries as text secrets in one transaction,
// recording history like SetSecret. Existing names are left alone under
// StrategySkip and replaced under StrategyOverwrite.
func (u *Unlocked) ApplyImport(ctx context.Context, envID domain.ID[domain.Environment], plan ImportPlan, s Strategy) error {
	if s != StrategySkip && s != StrategyOverwrite {
		return fmt.Errorf("unknown import strategy")
	}
	return u.store.Tx(ctx, func(tx RecordStore) error {
		key, err := u.envKey(ctx, tx, envID)
		if err != nil {
			return err
		}
		defer zeroKey(&key)
		existing, err := u.loadSecrets(ctx, tx, key, envID)
		if err != nil {
			return err
		}
		for _, e := range plan.entries {
			if _, ok := findSecret(existing, e.Name); ok && s == StrategySkip {
				continue
			}
			if _, err := u.setSecretTx(ctx, tx, key, envID, e.Name, e.Value, domain.TypeText, ""); err != nil {
				return err
			}
		}
		return nil
	})
}

// Export writes the named secrets (all when names is empty) in format f.
// A requested name that does not exist is domain.ErrNotFound.
func (u *Unlocked) Export(ctx context.Context, envID domain.ID[domain.Environment], names []domain.SecretName, f importexport.Format, w io.Writer) error {
	secrets, err := u.ListSecrets(ctx, envID)
	if err != nil {
		return err
	}
	defer func() {
		for _, s := range secrets {
			clear(s.Value)
		}
	}()
	picked := secrets
	if len(names) > 0 {
		picked = nil
		for _, n := range names {
			s, ok := findSecret(secrets, n)
			if !ok {
				return fmt.Errorf("secret: %w", domain.ErrNotFound)
			}
			picked = append(picked, s)
		}
	}
	entries := make([]importexport.Entry, len(picked))
	for i, s := range picked {
		entries[i] = importexport.Entry{Name: s.Name, Value: s.Value}
	}
	return importexport.Write(f, w, entries)
}
