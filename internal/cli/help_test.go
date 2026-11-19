package cli

import (
	"strings"
	"testing"
)

// TestHelpKeyFacts pins the facts each --help text must state, so the help
// cannot drift away from the behaviour documented in the README.
func TestHelpKeyFacts(t *testing.T) {
	cases := []struct {
		args  []string
		facts []string
	}{
		{[]string{"--help"}, []string{".secrio.toml", "searched upward", "SECRIO_HOME", "SECRIO_PASSWORD",
			"SECRIO_VAULT", "SECRIO_PROJECT", "SECRIO_ENV", "flags, then", "125", "126", "127", "128+N", "Example"}},
		{[]string{"run", "--help"}, []string{"125", "126", "127", "128+N", "SECRIO_PASSWORD", "SIGINT", "no terminal", "Put -- before", "flag-parse error", "Example"}},
		{[]string{"set", "--help"}, []string{"masked prompt", "stdin", "1 MiB when read from stdin", "CRLF", "NUL", "empty", "Example"}},
		{[]string{"get", "--help"}, []string{"raw", "terminal", "Example"}},
		{[]string{"unlock", "--help"}, []string{"12h", "1h", "--idle 0", "Example"}},
		{[]string{"list", "--help"}, []string{"projects", "environments", "secret names", "never values", "Example"}},
		{[]string{"vault", "create", "--help"}, []string{"letters", "SECRIO_PASSWORD", "case", "Example"}},
	}
	for _, c := range cases {
		e := newEnv(t)
		if code := e.run(nil, c.args...); code != ExitOK {
			t.Errorf("%v: code=%d err=%q", c.args, code, e.err)
			continue
		}
		for _, f := range c.facts {
			if !strings.Contains(e.out.String(), f) {
				t.Errorf("%v: help lacks %q", c.args, f)
			}
		}
	}
}
