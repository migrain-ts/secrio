package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	prefsFile    = "desktop.json"
	prefsMaxSize = 4 << 10
)

type prefs struct {
	LastVault string `json:"lastVault"`
}

// loadPrefs reads desktop.json; a missing, corrupt or oversized file means no
// preference.
func loadPrefs(dir string) prefs {
	path := filepath.Join(dir, prefsFile)
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > prefsMaxSize {
		return prefs{}
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) > prefsMaxSize {
		return prefs{}
	}
	var p prefs
	if json.Unmarshal(b, &p) != nil {
		return prefs{}
	}
	return p
}

// savePrefs writes desktop.json atomically (temp file + rename), mode 0600.
func savePrefs(dir string, p prefs) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, prefsFile+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(dir, prefsFile)); err != nil {
		return err
	}
	ok = true
	return nil
}
