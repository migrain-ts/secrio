package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"secrio/internal/domain"
)

func newEnvCmd(d Deps, flags *Flags) *cobra.Command {
	envCmd := &cobra.Command{Use: "env", Short: "Manage environments"}
	envCmd.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create an environment in the context project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := LoadContext(d, *flags)
			if err != nil {
				return err
			}
			projectName, err := c.NeedProject()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			u, err := openVaultFromContext(ctx, d, c)
			if err != nil {
				return err
			}
			defer u.Close()
			p, err := resolveProject(ctx, u, projectName)
			if err != nil {
				return err
			}
			if _, err := u.CreateEnvironment(ctx, p.ID, args[0]); err != nil {
				if errors.Is(err, domain.ErrDuplicate) {
					return fmt.Errorf("environment %q already exists", args[0])
				}
				return err
			}
			fmt.Fprintf(d.Out, "created environment %s\n", args[0])
			return nil
		},
	})
	return envCmd
}
