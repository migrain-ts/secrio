package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

// contextFile is the name of the per-directory context file.
const contextFile = ".secrio.toml"

// Flags holds the values of the global context flags.
type Flags struct{ Vault, Project, Env string }

// Context is the resolved vault/project/env selection (names, not ids).
type Context struct{ Vault, Project, Env string }

type fileContext struct {
	Vault   string `toml:"vault"`
	Project string `toml:"project"`
	Env     string `toml:"env"`
}

// addContextFlags registers --vault, --project and --env as persistent flags.
func addContextFlags(cmd *cobra.Command, f *Flags) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&f.Vault, "vault", "", "vault name (env SECRIO_VAULT)")
	pf.StringVar(&f.Project, "project", "", "project name (env SECRIO_PROJECT)")
	pf.StringVar(&f.Env, "env", "", "environment name (env SECRIO_ENV)")
}

// LoadContext resolves each field independently: flags > SECRIO_* env vars >
// nearest .secrio.toml (searched from d.Getwd upwards). Empty means unset.
func LoadContext(d Deps, f Flags) (Context, error) {
	c := Context{
		Vault:   first(f.Vault, d.Getenv("SECRIO_VAULT")),
		Project: first(f.Project, d.Getenv("SECRIO_PROJECT")),
		Env:     first(f.Env, d.Getenv("SECRIO_ENV")),
	}
	if c.Vault != "" && c.Project != "" && c.Env != "" {
		return c, nil
	}
	fc, err := findContextFile(d)
	if err != nil {
		return Context{}, err
	}
	c.Vault = first(c.Vault, fc.Vault)
	c.Project = first(c.Project, fc.Project)
	c.Env = first(c.Env, fc.Env)
	return c, nil
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// findContextFile returns the nearest .secrio.toml in the working directory
// or above; the first one found wins. No file yields the zero value.
func findContextFile(d Deps) (fileContext, error) {
	dir, err := d.Getwd()
	if err != nil {
		return fileContext{}, err
	}
	for {
		path := filepath.Join(dir, contextFile)
		data, err := os.ReadFile(path)
		if err == nil {
			return parseContextFile(path, data)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fileContext{}, err // *PathError names the path, no content
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fileContext{}, nil
		}
		dir = parent
	}
}

// parseContextFile decodes strictly. Errors name the path and, at most, the
// parser's message with line/column or the offending key; never file content.
func parseContextFile(path string, data []byte) (fileContext, error) {
	var fc fileContext
	err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&fc)
	if err == nil {
		return fc, nil
	}
	var sm *toml.StrictMissingError
	if errors.As(err, &sm) && len(sm.Errors) > 0 {
		key := sm.Errors[0].Key()
		return fileContext{}, fmt.Errorf("invalid %s: unknown key %q", path, key[len(key)-1])
	}
	var de *toml.DecodeError
	if errors.As(err, &de) {
		row, col := de.Position()
		return fileContext{}, fmt.Errorf("invalid %s: %s (line %d, column %d)", path, de.Error(), row, col)
	}
	return fileContext{}, fmt.Errorf("invalid %s: %v", path, err)
}

func needErr(what, flag, env string) error {
	return UsageErr("no %s: use --%s, %s or %s", what, flag, env, contextFile)
}

// NeedVault returns the vault name or a usage error saying how to set it.
func (c Context) NeedVault() (string, error) {
	if c.Vault == "" {
		return "", needErr("vault", "vault", "SECRIO_VAULT")
	}
	return c.Vault, nil
}

// NeedProject returns the project name or a usage error.
func (c Context) NeedProject() (string, error) {
	if c.Project == "" {
		return "", needErr("project", "project", "SECRIO_PROJECT")
	}
	return c.Project, nil
}

// NeedEnv returns the environment name or a usage error.
func (c Context) NeedEnv() (string, error) {
	if c.Env == "" {
		return "", needErr("env", "env", "SECRIO_ENV")
	}
	return c.Env, nil
}
