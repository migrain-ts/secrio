// Package cli is the Cobra command layer of the secrio binary. All I/O and
// side effects go through Deps; it holds no business logic.
package cli

import (
	"errors"
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

	usageArgs(root)
	return root
}

// usageArgs makes every argument-validation error exit 2, for all commands
// in the tree (including ones added by later tasks). Groups (commands with
// subcommands and no Args) reject positional arguments as unknown commands;
// leaves without Args keep Cobra's default; errors already carrying an
// *ExitError keep their code.
func usageArgs(cmd *cobra.Command) {
	switch {
	case cmd.Args == nil && cmd.HasSubCommands():
		cmd.Args = func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageErr("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return nil
		}
		if !cmd.Runnable() {
			// A non-runnable command short-circuits to help before Args runs.
			cmd.RunE = func(c *cobra.Command, _ []string) error { return c.Help() }
		}
	case cmd.Args != nil:
		inner := cmd.Args
		cmd.Args = func(c *cobra.Command, args []string) error {
			err := inner(c, args)
			var ee *ExitError
			if err == nil || errors.As(err, &ee) {
				return err
			}
			return UsageErr("%v", err)
		}
	}
	for _, c := range cmd.Commands() {
		usageArgs(c)
	}
}

// Execute runs the CLI and returns the process exit code.
func Execute(d Deps, args []string) int {
	if args == nil {
		args = []string{} // never let Cobra fall back to os.Args
	}
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
