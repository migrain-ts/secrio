package cli

import (
	"context"
	"errors"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"secrio/internal/app"
)

const runUsage = "usage: secrio run [flags] -- <command> [args...]"

// testHookRunVault, when set by tests, receives the vault handle opened by
// `run` (to check it is closed before the command starts).
var testHookRunVault func(*app.Unlocked)

// withoutPassword returns environ without SECRIO_PASSWORD: the vault
// password must never reach the child.
func withoutPassword(environ []string) []string {
	const key = "SECRIO_PASSWORD="
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if len(kv) >= len(key) && (kv[:len(key)] == key ||
			runtime.GOOS == "windows" && strings.EqualFold(kv[:len(key)], key)) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// runEnvironment builds the child's environment and closes the vault before
// returning, so the vault key is not held while the command runs.
func runEnvironment(ctx context.Context, d Deps, flags *Flags) ([]string, error) {
	u, en, err := openSecretEnv(ctx, d, flags)
	if err != nil {
		return nil, err
	}
	defer u.Close()
	if testHookRunVault != nil {
		testHookRunVault(u)
	}
	return u.ProcessEnv(ctx, en.ID, withoutPassword(d.Environ()))
}

func newRunCmd(d Deps, flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "run [flags] -- <command> [args...]",
		Short: "Run a command with the environment's secrets as environment variables",
		Args: func(cmd *cobra.Command, args []string) error {
			if dash := cmd.ArgsLenAtDash(); dash != 0 || len(args) == 0 {
				return &ExitError{Code: ExitRunFailure, Err: errors.New(runUsage)}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := runEnvironment(cmd.Context(), d, flags)
			if err != nil {
				// Every pre-launch failure is 125, usage errors included: exit 2
				// would collide with a child's own exit 2.
				return &ExitError{Code: ExitRunFailure, Err: err}
			}
			code, err := d.Run(cmd.Context(), args[0], args[1:], env, d.In, d.Out, d.Err)
			if err != nil || code != ExitOK {
				return &ExitError{Code: code, Err: err} // Err nil: nothing printed
			}
			return nil
		},
	}
}
