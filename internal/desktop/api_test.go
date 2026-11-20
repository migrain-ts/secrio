package desktop

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"testing"
)

// boundAllowlist is every method the webview may call. Adding a bound method
// means adding it to API AND here.
var boundAllowlist = []string{
	"ApplyImport",
	"CopySecret",
	"CreateEnvironment",
	"CreateProject",
	"CreateVault",
	"CurrentVault",
	"DeleteEnvironment",
	"DeleteProject",
	"DeleteSecret",
	"ListEnvironments",
	"ListProjects",
	"ListSecrets",
	"ListVaults",
	"Lock",
	"PickEnvFile",
	"PreviewImport",
	"RevealSecret",
	"SaveSecret",
	"SelectVault",
	"Unlock",
}

func exportedMethods(t reflect.Type) []string {
	var out []string
	for i := range t.NumMethod() {
		out = append(out, t.Method(i).Name)
	}
	slices.Sort(out)
	return out
}

func TestAPIMethodSetIsAllowlist(t *testing.T) {
	got := exportedMethods(reflect.TypeFor[*API]())
	want := slices.Sorted(slices.Values(boundAllowlist))
	if !slices.Equal(got, want) {
		t.Fatalf("API methods = %v, want %v", got, want)
	}
	if slices.Contains(got, "Shutdown") {
		t.Fatal("Shutdown must not be bound")
	}
}

// The committed bindings must export exactly the bound methods, so a change
// to API cannot ship with stale JS.
func TestBindingsMatchAPI(t *testing.T) {
	b, err := os.ReadFile("../../desktop/frontend/wailsjs/go/desktop/API.d.ts")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range regexp.MustCompile(`(?m)^export function (\w+)\(`).FindAllStringSubmatch(string(b), -1) {
		got = append(got, m[1])
	}
	slices.Sort(got)
	if want := exportedMethods(reflect.TypeFor[*API]()); !slices.Equal(got, want) {
		t.Fatalf("wailsjs bindings export %v, API has %v: run scripts/desktop-bindings.sh", got, want)
	}
}

// funcSig drops the receiver from a method type.
func funcSig(m reflect.Method) (in, out []reflect.Type) {
	ft := m.Type
	for i := 1; i < ft.NumIn(); i++ {
		in = append(in, ft.In(i))
	}
	for i := range ft.NumOut() {
		out = append(out, ft.Out(i))
	}
	return in, out
}

// reshaped are forwarders whose results differ from App's because Wails v2
// cannot return them as is; their inputs must still match.
var reshaped = map[string]bool{"CurrentVault": true}

// Wails v2 binds (), (T), (error) and (T, error) only; with more results the
// JS promise resolves to null and the error is lost.
func TestAPIResultsFitWails(t *testing.T) {
	errT := reflect.TypeFor[error]()
	apiT := reflect.TypeFor[*API]()
	for i := range apiT.NumMethod() {
		m := apiT.Method(i)
		n := m.Type.NumOut()
		if n > 2 || (n == 2 && m.Type.Out(1) != errT) {
			t.Errorf("%s: results %v not supported by Wails", m.Name, m.Type)
		}
	}
}

func TestAPISignaturesMatchApp(t *testing.T) {
	apiT, appT := reflect.TypeFor[*API](), reflect.TypeFor[*App]()
	for _, name := range boundAllowlist {
		am, ok := apiT.MethodByName(name)
		if !ok {
			t.Errorf("API lacks %s", name)
			continue
		}
		pm, ok := appT.MethodByName(name)
		if !ok {
			t.Errorf("App lacks %s", name)
			continue
		}
		ai, ao := funcSig(am)
		pi, po := funcSig(pm)
		if reshaped[name] {
			ao, po = nil, nil
		}
		if !slices.Equal(ai, pi) || !slices.Equal(ao, po) {
			t.Errorf("%s: API %v, App %v", name, am.Type, pm.Type)
		}
	}
}

func TestAPIForwardsToApp(t *testing.T) {
	env := newTestApp(t)
	api := NewAPI(env.app)
	vs, err := api.ListVaults()
	if err != nil || len(vs) != 0 {
		t.Fatalf("ListVaults = %v, %v; want empty", vs, err)
	}
	v, err := api.CreateVault("work", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	vs, err = api.ListVaults()
	if err != nil || len(vs) != 1 || vs[0] != v {
		t.Fatalf("ListVaults = %v, %v; want [%v]", vs, err, v)
	}
	cur, err := api.CurrentVault()
	if err != nil || cur != (CurrentVaultDTO{Vault: v, Unlocked: true}) {
		t.Fatalf("CurrentVault = %+v, %v", cur, err)
	}
	p, err := api.CreateProject("hotenb")
	if err != nil {
		t.Fatal(err)
	}
	if ps, err := api.ListProjects(); err != nil || len(ps) != 1 || ps[0].ID != p.ID {
		t.Fatalf("ListProjects = %v, %v", ps, err)
	}
	if err := api.Lock(); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ListProjects(); codeOf(t, err) != CodeLocked {
		t.Fatalf("after Lock: %v", err)
	}
	if cur, err := api.CurrentVault(); err != nil || cur != (CurrentVaultDTO{Vault: v}) {
		t.Fatalf("CurrentVault after Lock = %+v, %v", cur, err)
	}
}

func TestAPICurrentVaultNone(t *testing.T) {
	cur, err := NewAPI(newTestApp(t).app).CurrentVault()
	if err != nil || cur != (CurrentVaultDTO{}) {
		t.Fatalf("CurrentVault = %+v, %v", cur, err)
	}
}
