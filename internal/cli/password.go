package cli

import (
	"bytes"
	"errors"
	"fmt"

	"secrio/internal/domain"
)

// envPasswordErr tells the user a rejected password came from SECRIO_PASSWORD
// (not from a prompt). It never includes the password.
func envPasswordErr(d Deps, err error) error {
	if errors.Is(err, domain.ErrInvalidPassword) && d.Getenv("SECRIO_PASSWORD") != "" {
		return fmt.Errorf("%w (from SECRIO_PASSWORD)", err)
	}
	return err
}

// readPassword returns the vault password. SECRIO_PASSWORD wins when set (no
// prompt); otherwise a terminal is required. With confirm the password is
// asked twice and a mismatch is refused. The caller should clear the result.
func readPassword(d Deps, prompt string, confirm bool) ([]byte, error) {
	if env := d.Getenv("SECRIO_PASSWORD"); env != "" {
		return []byte(env), nil
	}
	if !d.StdinTTY() {
		return nil, LockedErr(errors.New("no terminal and SECRIO_PASSWORD is not set"))
	}
	pw, err := d.ReadPassword(prompt)
	if err != nil {
		clear(pw)
		return nil, err
	}
	if !confirm {
		return pw, nil
	}
	again, err := d.ReadPassword("Confirm password: ")
	defer clear(again)
	if err != nil {
		clear(pw)
		return nil, err
	}
	if !bytes.Equal(pw, again) {
		clear(pw)
		return nil, errors.New("passwords do not match")
	}
	return pw, nil
}
