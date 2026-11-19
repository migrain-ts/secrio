// Package cli is the Cobra command layer of the secrio binary. All I/O and
// side effects go through Deps; it holds no business logic.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Flags holds the global context flags. Task 2 moves this type.
type Flags struct{ Vault, Project, Env string }

// NewRoot builds the root command with every subcommand attached.
func NewRoot(d Deps) *cobra.Command {
	var flags Flags
	_ = &flags // handed to the commands below as they are added

	root := &cobra.Command{
		Use:           "secrio",
		Short:         "Local-first secrets manager",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageErr("unknown command %q", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetOut(d.Out)
	root.SetErr(d.Err)
	root.SetVersionTemplate("secrio {{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return UsageErr("%v", err)
	})

	// Subcommands: one constructor call per line, appended by later tasks.

	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute(d Deps, args []string) int {
	return run(d, NewRoot(d), args)
}

func run(d Deps, root *cobra.Command, args []string) int {
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		fmt.Fprintf(d.Err, "secrio: %s\n", err)
	}
	return exitCode(err)
}
