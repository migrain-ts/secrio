package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newProjectCmd(d Deps, flags *Flags) *cobra.Command {
	project := &cobra.Command{Use: "project", Short: "Manage projects"}
	project.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create a project in the context vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			u, err := openContextVault(ctx, d, flags)
			if err != nil {
				return err
			}
			defer u.Close()
			if _, err := u.CreateProject(ctx, args[0], ""); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "created project %s\n", args[0])
			return nil
		},
	})
	return project
}
