package desktop

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"secrio/internal/domain"
)

func secretEnv(t *testing.T) (*testEnv, string) {
	t.Helper()
	env := newTestApp(t)
	mustCreate(t, env, "work")
	p, err := env.app.CreateProject("web")
	if err != nil {
		t.Fatal(err)
	}
	e, err := env.app.CreateEnvironment(p.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	return env, e.ID
}

func listCount(t *testing.T, env *testEnv, envID string) int {
	t.Helper()
	l, err := env.app.ListSecrets(envID)
	if err != nil {
		t.Fatal(err)
	}
	return len(l)
}

// coreGet reads a secret straight through the Core.
func coreGet(t *testing.T, env *testEnv, envID, name string) domain.Secret {
	t.Helper()
	svc, err := env.app.service()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, err := svc.Open(ctx, env.app.getCurrent())
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	s, err := u.GetSecret(ctx, domain.ID[domain.Environment](envID), domain.SecretName(name))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestListSecretsHasNoValue(t *testing.T) {
	env, id := secretEnv(t)
	seed(t, env, id, "B_ONE")
	seed(t, env, id, "A_ONE")
	seed(t, env, id, "a_low")
	l, err := env.app.ListSecrets(id)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range l {
		names = append(names, s.Name)
	}
	if !sort.StringsAreSorted(names) || len(names) != 3 {
		t.Fatalf("names %v", names)
	}
	b, _ := json.Marshal(l)
	if strings.Contains(string(b), seedValue) {
		t.Fatal("value in JSON")
	}
	var raw []map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range raw[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"createdAt", "description", "name", "type", "updatedAt"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys %v", keys)
	}
	if l[0].Type != "text" || l[0].CreatedAt != "2026-11-19T12:00:00Z" {
		t.Fatalf("%+v", l[0])
	}
}

func TestListSecretsEmptyNonNil(t *testing.T) {
	env, id := secretEnv(t)
	l, err := env.app.ListSecrets(id)
	if err != nil || l == nil || len(l) != 0 {
		t.Fatalf("%v %v", l, err)
	}
}

func TestSaveRevealRoundTrip(t *testing.T) {
	env, id := secretEnv(t)
	for _, v := range []string{"abc", "a=b", "l1\nl2", "é", "x\r\n", strings.Repeat("A", 1<<20)} {
		if err := env.app.SaveSecret(id, "K", v, ""); err != nil {
			t.Fatal(err)
		}
		got, err := env.app.RevealSecret(id, "K")
		if err != nil || got != v {
			t.Fatalf("round trip failed for len %d: %v", len(v), err)
		}
	}
}

func TestSaveRejectsBadInput(t *testing.T) {
	env, id := secretEnv(t)
	const val = "v4lue-SHOULD-NOT-LEAK"
	cases := []struct {
		name, secret, value, typ, code string
	}{
		{"empty value", "K", "", "", CodeInvalidValue},
		{"nul", "K", "a\x00b" + val, "", CodeInvalidValue},
		{"too big", "K", strings.Repeat("A", 1<<20+1), "", CodeInvalidValue},
		{"bad type", "K", val, "bogus", CodeInvalidValue},
		{"name 1BAD", "1BAD", val, "", CodeInvalidName},
		{"name a-b", "a-b", val, "", CodeInvalidName},
		{"name empty", "", val, "", CodeInvalidName},
	}
	for _, c := range cases {
		err := env.app.SaveSecret(id, c.secret, c.value, c.typ)
		wantCode(t, err, c.code)
		if strings.Contains(err.Error(), val) || strings.Contains(err.Error(), testPassword) {
			t.Fatalf("%s: leak in %q", c.name, err)
		}
	}
	if n := listCount(t, env, id); n != 0 {
		t.Fatalf("stored %d", n)
	}
}

func TestSaveTypes(t *testing.T) {
	env, id := secretEnv(t)
	a := env.app
	if err := a.SaveSecret(id, "K", "v", ""); err != nil {
		t.Fatal(err)
	}
	if got := coreGet(t, env, id, "K").Type; got != domain.TypeText {
		t.Fatal(got)
	}
	if err := a.SaveSecret(id, "K", "v", "api_key"); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSecret(id, "K", "w", ""); err != nil {
		t.Fatal(err)
	}
	if got := coreGet(t, env, id, "K").Type; got != domain.TypeAPIKey {
		t.Fatalf("type not kept: %s", got)
	}
	for _, ty := range []string{"text", "password", "api_key", "token", "url", "private_key", "certificate", "json"} {
		if err := a.SaveSecret(id, "T_"+ty, "v", ty); err != nil {
			t.Fatalf("%s: %v", ty, err)
		}
	}
}

func TestSaveUpdateKeepsDescriptionAndCreatedAt(t *testing.T) {
	env, id := secretEnv(t)
	svc, _ := env.app.service()
	ctx := context.Background()
	u, err := svc.Open(ctx, env.app.getCurrent())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.SetSecret(ctx, domain.ID[domain.Environment](id), "K", []byte("v1"), domain.TypeToken, "my desc"); err != nil {
		t.Fatal(err)
	}
	u.Close()
	before := coreGet(t, env, id, "K")
	env.fake.Advance(10 * time.Minute) // under the 1h idle timeout
	if err := env.app.SaveSecret(id, "K", "v2", ""); err != nil {
		t.Fatal(err)
	}
	after := coreGet(t, env, id, "K")
	if after.Description != "my desc" || after.Type != domain.TypeToken {
		t.Fatalf("%+v", after)
	}
	if !after.CreatedAt.Equal(before.CreatedAt) || !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("times %v %v", after.CreatedAt, after.UpdatedAt)
	}
	if string(after.Value) != "v2" {
		t.Fatal("value not updated")
	}
	l, _ := env.app.ListSecrets(id)
	if l[0].UpdatedAt == l[0].CreatedAt {
		t.Fatal("UpdatedAt unchanged in DTO")
	}
}

func TestDeleteThenReveal(t *testing.T) {
	env, id := secretEnv(t)
	if err := env.app.SaveSecret(id, "K", "v", ""); err != nil {
		t.Fatal(err)
	}
	if err := env.app.DeleteSecret(id, "K"); err != nil {
		t.Fatal(err)
	}
	_, err := env.app.RevealSecret(id, "K")
	wantCode(t, err, CodeNotFound)
	wantCode(t, env.app.DeleteSecret(id, "K"), CodeNotFound)
	wantCode(t, env.app.DeleteSecret(id, "1BAD"), CodeInvalidName)
}

func TestSecretsLockedAndUnknownEnv(t *testing.T) {
	env, id := secretEnv(t)
	a := env.app
	if err := a.SaveSecret(id, "K", "v", ""); err != nil {
		t.Fatal(err)
	}
	_, err := a.ListSecrets("nope")
	wantCode(t, err, CodeNotFound)
	wantCode(t, a.SaveSecret("nope", "K", "v", ""), CodeNotFound)
	wantCode(t, a.DeleteSecret("nope", "K"), CodeNotFound)
	_, err = a.RevealSecret("nope", "K")
	wantCode(t, err, CodeNotFound)

	if err := a.Lock(); err != nil {
		t.Fatal(err)
	}
	_, err = a.ListSecrets(id)
	wantCode(t, err, CodeLocked)
	wantCode(t, a.SaveSecret(id, "K", "v", ""), CodeLocked)
	wantCode(t, a.DeleteSecret(id, "K"), CodeLocked)
	_, err = a.RevealSecret(id, "K")
	wantCode(t, err, CodeLocked)
}

func TestSecretErrorsNeverLeak(t *testing.T) {
	env, id := secretEnv(t)
	a := env.app
	const val = "v4lue-SHOULD-NOT-LEAK"
	const name = "SECRET_NAME_X"
	var errs []error
	_, e1 := a.RevealSecret(id, name)
	_, e2 := a.RevealSecret("nope", name)
	errs = append(errs, e1, e2,
		a.SaveSecret("nope", name, val, ""),
		a.SaveSecret(id, name, val, "bogus"),
		a.SaveSecret(id, "1"+name, val, ""),
		a.SaveSecret(id, name, "", ""),
		a.DeleteSecret(id, name),
		a.DeleteSecret("nope", name))
	_ = a.Lock()
	_, e3 := a.RevealSecret(id, name)
	errs = append(errs, e3, a.SaveSecret(id, name, val, ""), a.DeleteSecret(id, name))
	for i, err := range errs {
		if err == nil {
			t.Fatalf("case %d: nil error", i)
		}
		s := err.Error()
		if strings.Contains(s, val) || strings.Contains(s, testPassword) || strings.Contains(s, name) {
			t.Fatalf("case %d leaks: %s", i, s)
		}
	}
}
