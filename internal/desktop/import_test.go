package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"secrio/internal/app/importexport"
)

type fakeDialogs struct{ path string }

func (f *fakeDialogs) OpenFile(string) (string, error) { return f.path, nil }

type importFx struct {
	env *testEnv
	dlg *fakeDialogs
	eid string
}

func newImportFx(t *testing.T) importFx {
	t.Helper()
	dlg := &fakeDialogs{}
	env := newTestAppWith(t, func(d *Deps) { d.Dialogs = dlg })
	mustCreate(t, env, "work")
	p, err := env.app.CreateProject("web")
	if err != nil {
		t.Fatal(err)
	}
	e, err := env.app.CreateEnvironment(p.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.app.SaveSecret(e.ID, "OLD", "old-value", "password"); err != nil {
		t.Fatal(err)
	}
	return importFx{env: env, dlg: dlg, eid: e.ID}
}

func (f importFx) write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f importFx) pick(t *testing.T, p string) {
	t.Helper()
	f.dlg.path = p
	got, err := f.env.app.PickEnvFile()
	if err != nil || got != p {
		t.Fatalf("pick: %q %v", got, err)
	}
}

func (f importFx) count(t *testing.T) int {
	t.Helper()
	ss, err := f.env.app.ListSecrets(f.eid)
	if err != nil {
		t.Fatal(err)
	}
	return len(ss)
}

const baseEnv = "OLD=" + seedValue + "\nNEWB=v2\nNEWA=v1\n"

func TestPreviewNamesOnly(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	pv, err := f.env.app.PreviewImport(f.eid, p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pv.New, ",") != "NEWA,NEWB" || strings.Join(pv.Duplicate, ",") != "OLD" {
		t.Fatalf("%+v", pv)
	}
	b, _ := json.Marshal(pv)
	if strings.Contains(string(b), seedValue) || strings.Contains(string(b), "v1") {
		t.Fatalf("value in dto: %s", b)
	}
	if f.count(t) != 1 {
		t.Fatal("preview must not import")
	}
	// non-nil empty slices
	p2 := f.write(t, "")
	f.pick(t, p2)
	pv, err = f.env.app.PreviewImport(f.eid, p2)
	if err != nil || pv.New == nil || pv.Duplicate == nil {
		t.Fatalf("%+v %v", pv, err)
	}
}

func TestApplySkipAndOverwrite(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, "OLD=fresh\nNEWA=v1\n")
	f.pick(t, p)
	a := f.env.app
	if _, err := a.PreviewImport(f.eid, p); err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyImport(f.eid, p, "skip"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.RevealSecret(f.eid, "OLD"); v != "old-value" {
		t.Fatalf("skip changed value: %q", v)
	}
	if v, _ := a.RevealSecret(f.eid, "NEWA"); v != "v1" {
		t.Fatalf("new not imported: %q", v)
	}
	// stored hash was consumed
	wantCode(t, a.ApplyImport(f.eid, p, "overwrite"), CodeConflict)
	if _, err := a.PreviewImport(f.eid, p); err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyImport(f.eid, p, "overwrite"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.RevealSecret(f.eid, "OLD"); v != "fresh" {
		t.Fatalf("overwrite: %q", v)
	}
	ss, _ := a.ListSecrets(f.eid)
	for _, s := range ss {
		if s.Name == "OLD" && s.Type != "password" {
			t.Fatalf("type lost: %q", s.Type)
		}
	}
}

func TestApplyWithoutPreviewConflict(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	wantCode(t, f.env.app.ApplyImport(f.eid, p, "skip"), CodeConflict)
	if f.count(t) != 1 {
		t.Fatal("imported")
	}
}

func TestApplyConflictOnFileChange(t *testing.T) {
	for name, edit := range map[string]string{"add": baseEnv + "EXTRA=x\n", "remove": "OLD=a\nNEWA=v1\n"} {
		t.Run(name, func(t *testing.T) {
			f := newImportFx(t)
			p := f.write(t, baseEnv)
			f.pick(t, p)
			if _, err := f.env.app.PreviewImport(f.eid, p); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(edit), 0o600); err != nil {
				t.Fatal(err)
			}
			wantCode(t, f.env.app.ApplyImport(f.eid, p, "overwrite"), CodeConflict)
			if f.count(t) != 1 {
				t.Fatal("imported")
			}
			// hash dropped on conflict
			wantCode(t, f.env.app.ApplyImport(f.eid, p, "overwrite"), CodeConflict)
		})
	}
}

func TestApplyConflictOnVaultChange(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	if _, err := f.env.app.PreviewImport(f.eid, p); err != nil {
		t.Fatal(err)
	}
	if err := f.env.app.SaveSecret(f.eid, "NEWA", "other", ""); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.env.app.ApplyImport(f.eid, p, "skip"), CodeConflict)
	if f.count(t) != 2 {
		t.Fatal("imported")
	}
}

func TestInvalidStrategy(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	if _, err := f.env.app.PreviewImport(f.eid, p); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "merge", "SKIP"} {
		wantCode(t, f.env.app.ApplyImport(f.eid, p, s), CodeInvalidValue)
	}
	// the stored preview survives a bad strategy
	if err := f.env.app.ApplyImport(f.eid, p, "skip"); err != nil {
		t.Fatal(err)
	}
}

func TestNotPickedPathRefusedWithoutTouchingFS(t *testing.T) {
	f := newImportFx(t)
	missing := filepath.Join(t.TempDir(), "nope.env")
	_, err := f.env.app.PreviewImport(f.eid, missing)
	wantCode(t, err, CodeInvalidValue)
	wantCode(t, f.env.app.ApplyImport(f.eid, missing, "skip"), CodeInvalidValue)

	picked := f.write(t, baseEnv)
	f.pick(t, picked)
	other := f.write(t, baseEnv)
	_, err = f.env.app.PreviewImport(f.eid, other)
	wantCode(t, err, CodeInvalidValue)
	wantCode(t, f.env.app.ApplyImport(f.eid, other, "skip"), CodeInvalidValue)

	dotted := filepath.Dir(picked) + "/x/../" + filepath.Base(picked) // Join would clean it
	_, err = f.env.app.PreviewImport(f.eid, dotted)
	wantCode(t, err, CodeInvalidValue)
}

func TestPickCancelledAndNilDialogs(t *testing.T) {
	f := newImportFx(t)
	f.dlg.path = ""
	got, err := f.env.app.PickEnvFile()
	if got != "" || err != nil {
		t.Fatalf("%q %v", got, err)
	}
	_, err = f.env.app.PreviewImport(f.eid, "")
	wantCode(t, err, CodeInvalidValue)

	env := newTestApp(t)
	_, err = env.app.PickEnvFile()
	wantCode(t, err, CodeUnsupported)
}

func TestPickedSetBounded(t *testing.T) {
	f := newImportFx(t)
	first := f.write(t, baseEnv)
	f.pick(t, first)
	for i := 0; i < pickedCap; i++ {
		f.pick(t, f.write(t, baseEnv))
	}
	_, err := f.env.app.PreviewImport(f.eid, first)
	wantCode(t, err, CodeInvalidValue)
}

func TestInvalidFileContent(t *testing.T) {
	f := newImportFx(t)
	for _, c := range []string{"bad-key=SECRETVAL\n", "KEY=\"unterminated SECRETVAL\n", "no equals SECRETVAL\n"} {
		p := f.write(t, c)
		f.pick(t, p)
		_, err := f.env.app.PreviewImport(f.eid, p)
		wantCode(t, err, CodeInvalidValue)
		for _, bad := range []string{"SECRETVAL", "bad-key", "unterminated", "equals"} {
			if strings.Contains(err.Error(), bad) {
				t.Fatalf("leak %q in %v", bad, err)
			}
		}
	}
}

func TestFileOverCap(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, "K="+strings.Repeat("a", importexport.MaxInputBytes))
	f.pick(t, p)
	_, err := f.env.app.PreviewImport(f.eid, p)
	wantCode(t, err, CodeInvalidValue)
}

func TestMissingAndDirectory(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	_, err := f.env.app.PreviewImport(f.eid, p)
	wantCode(t, err, CodeNotFound)

	dir := t.TempDir()
	f.pick(t, dir)
	_, err = f.env.app.PreviewImport(f.eid, dir)
	wantCode(t, err, CodeInvalidValue)
}

func TestUnknownEnv(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	_, err := f.env.app.PreviewImport("nope", p)
	wantCode(t, err, CodeNotFound)
}

func TestImportLocked(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	if _, err := f.env.app.PreviewImport(f.eid, p); err != nil {
		t.Fatal(err)
	}
	if err := f.env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	f.pick(t, p) // Lock cleared the picked set; the vault is still locked
	_, err := f.env.app.PreviewImport(f.eid, p)
	wantCode(t, err, CodeLocked)
	wantCode(t, f.env.app.ApplyImport(f.eid, p, "skip"), CodeConflict) // preview gone
	// a preview that was stored while unlocked cannot be applied while locked
	f2 := newImportFx(t)
	p2 := f2.write(t, baseEnv)
	f2.pick(t, p2)
	if _, err := f2.env.app.PreviewImport(f2.eid, p2); err != nil {
		t.Fatal(err)
	}
	svc, _ := f2.env.app.service()
	if err := svc.Lock(t.Context(), f2.env.app.getCurrent()); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f2.env.app.ApplyImport(f2.eid, p2, "skip"), CodeLocked)
}

func TestLockClearsImportState(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	if _, err := f.env.app.PreviewImport(f.eid, p); err != nil {
		t.Fatal(err)
	}
	vid := f.env.app.getCurrent()
	if err := f.env.app.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := f.env.app.Unlock(vid, testPassword); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.env.app.ApplyImport(f.eid, p, "skip"), CodeInvalidValue)
	f.pick(t, p) // picked again but the preview is gone
	wantCode(t, f.env.app.ApplyImport(f.eid, p, "skip"), CodeConflict)
	if f.count(t) != 1 {
		t.Fatal("imported")
	}
}

func TestShutdownClearsImportState(t *testing.T) {
	f := newImportFx(t)
	p := f.write(t, baseEnv)
	f.pick(t, p)
	f.env.app.Shutdown()
	f.env.app.mu.Lock()
	defer f.env.app.mu.Unlock()
	if len(f.env.app.imp.picked) != 0 || len(f.env.app.imp.previews) != 0 {
		t.Fatal("not cleared")
	}
}

func TestPlanHashCanonical(t *testing.T) {
	if planHash([]string{"a", "bc"}, nil) == planHash([]string{"ab", "c"}, nil) {
		t.Fatal("collision")
	}
	if planHash([]string{"a"}, []string{"b"}) == planHash([]string{"b"}, []string{"a"}) {
		t.Fatal("sections not distinguished")
	}
	if planHash([]string{"a"}, nil) == planHash(nil, []string{"a"}) {
		t.Fatal("sections not distinguished")
	}
}
