package cli

import (
	"errors"
	"testing"
)

func TestReadPasswordEnvWinsAndIsCopy(t *testing.T) {
	e := newEnv(t)
	e.vars["SECRIO_PASSWORD"] = "abc"
	pw, err := readPassword(e.d, "Password: ", true)
	if err != nil || string(pw) != "abc" || e.pwCalls != 0 {
		t.Fatalf("pw=%q err=%v calls=%d", pw, err, e.pwCalls)
	}
	clear(pw)
	if e.vars["SECRIO_PASSWORD"] != "abc" {
		t.Fatal("env mutated")
	}
}

func TestReadPasswordNoTTY(t *testing.T) {
	e := newEnv(t)
	e.tty = false
	_, err := readPassword(e.d, "Password: ", false)
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitLocked || e.pwCalls != 0 {
		t.Fatalf("err=%v calls=%d", err, e.pwCalls)
	}
}

func TestReadPasswordPromptError(t *testing.T) {
	e := newEnv(t) // empty queue: ReadPassword fails
	if _, err := readPassword(e.d, "Password: ", false); err == nil {
		t.Fatal("want error")
	}
}
