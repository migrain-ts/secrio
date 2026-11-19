// Package cli is the Cobra command layer of the secrio binary. All I/O and
// side effects go through Deps; it holds no business logic.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// NewRoot builds the root command with every subcommand attached.
func NewRoot(d Deps) *cobra.Command {
	var flags Flags

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

	addContextFlags(root, &flags)

	// Subcommands: one constructor call per line, appended by later tasks.

	finalize(root)
	return root
}

// finalize adds Cobra's built-in help and completion commands (normally added
// lazily inside Execute, and only once the root has subcommands) and applies
// usageArgs to the whole tree. Idempotent; run calls it again so commands
// added after NewRoot are covered too.
func finalize(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	usageArgs(root)
}

const usageArgsMark = "secrio.usageargs"

// usageArgs makes every argument-validation error exit 2, for all commands
// in the tree (including ones added by later tasks). Groups (commands with
// subcommands and no Args) reject positional arguments as unknown commands;
// leaves without Args accept none (they must opt in with Args); errors
// already carrying an *ExitError keep their code. Idempotent.
func usageArgs(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		usageArgs(c)
	}
	if cmd.Annotations[usageArgsMark] != "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[usageArgsMark] = "1"

	switch {
	case cmd.Name() == "help" && cmd.Parent() != nil && cmd.Parent().Parent() == nil:
		return // Cobra's built-in help takes any command path
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
	default:
		if cmd.Args == nil {
			cmd.Args = cobra.NoArgs
		}
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
}

// Execute runs the CLI and returns the process exit code.
func Execute(d Deps, args []string) int {
	if args == nil {
		args = []string{} // never let Cobra fall back to os.Args
	}
	return run(d, NewRoot(d), args)
}

func run(d Deps, root *cobra.Command, args []string) int {
	finalize(root)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		fmt.Fprintf(d.Err, "secrio: %s\n", err)
	}
	return exitCode(err)
}
