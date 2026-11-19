package app

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"

	"secrio/internal/app/importexport"
	"secrio/internal/domain"
)

// closedCalls invokes every exported Unlocked method except Close.
func closedCalls(u *Unlocked) map[string]func() error {
	ctx := context.Background()
	const env domain.ID[domain.Environment] = "e"
	const proj domain.ID[domain.Project] = "p"
	return map[string]func() error{
		"CompareEnvs":       func() error { _, err := u.CompareEnvs(ctx, env); return err },
		"CreateEnvironment": func() error { _, err := u.CreateEnvironment(ctx, proj, "n"); return err },
		"ListEnvironments":  func() error { _, err := u.ListEnvironments(ctx, proj); return err },
		"DeleteEnvironment": func() error { return u.DeleteEnvironment(ctx, env) },
		"History":           func() error { _, err := u.History(ctx, env); return err },
		"RestoreVersion":    func() error { return u.RestoreVersion(ctx, "x", []byte("pw")) },
		"PlanImport":        func() error { _, err := u.PlanImport(ctx, env, nil); return err },
		"ApplyImport":       func() error { return u.ApplyImport(ctx, env, ImportPlan{}, StrategySkip) },
		"Export": func() error {
			return u.Export(ctx, env, nil, importexport.FormatDotenv, io.Discard)
		},
		"ProcessEnv":    func() error { _, err := u.ProcessEnv(ctx, env, nil); return err },
		"SetSecret":     func() error { _, err := u.SetSecret(ctx, env, "K", []byte("v"), "", ""); return err },
		"GetSecret":     func() error { _, err := u.GetSecret(ctx, env, "K"); return err },
		"ListSecrets":   func() error { _, err := u.ListSecrets(ctx, env); return err },
		"DeleteSecret":  func() error { return u.DeleteSecret(ctx, env, "K") },
		"CreateProject": func() error { _, err := u.CreateProject(ctx, "n", ""); return err },
		"ListProjects":  func() error { _, err := u.ListProjects(ctx); return err },
		"RenameProject": func() error { return u.RenameProject(ctx, proj, "n") },
		"DeleteProject": func() error { return u.DeleteProject(ctx, proj) },
		"DuplicateProject": func() error {
			_, err := u.DuplicateProject(ctx, proj, "n")
			return err
		},
	}
}

func TestClosedHandleReturnsErrLocked(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	u, err := h.svc.OpenWithPassword(context.Background(), id, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	calls := closedCalls(u)
	// every exported method but Close must be covered
	if got := reflect.TypeOf(u).NumMethod(); got != len(calls)+1 {
		t.Fatalf("%d exported methods, %d covered (+Close)", got, len(calls))
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, domain.ErrLocked) {
				t.Fatalf("got %v, want ErrLocked", err)
			}
		})
	}
	if err := u.Close(); err != nil { // double Close
		t.Fatal(err)
	}
}

func TestConcurrentCloseIsRaceFree(t *testing.T) {
	h := newHarness(t)
	id := h.newVault(t)
	u, err := h.svc.OpenWithPassword(context.Background(), id, []byte(testPw))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			if err := closedCalls(u)["ListProjects"](); err != nil && !errors.Is(err, domain.ErrLocked) {
				t.Errorf("got %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		u.Close()
	}()
	wg.Wait()
}
