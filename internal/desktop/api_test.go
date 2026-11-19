package desktop

import (
	"reflect"
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
	cur, unlocked, err := api.CurrentVault()
	if err != nil || cur != v || !unlocked {
		t.Fatalf("CurrentVault = %v, %v, %v", cur, unlocked, err)
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
}
