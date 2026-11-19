package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"secrio/internal/core"
)

func newVaultCmd(d Deps, _ *Flags) *cobra.Command {
	vault := &cobra.Command{Use: "vault", Short: "Manage vaults"}
	vault.AddCommand(newVaultCreateCmd(d), newVaultListCmd(d))
	return vault
}

func newVaultCreateCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new vault",
		Long: `Create a vault. The name has up to 64 letters, digits, '.', '_' or '-', and must not
start with '.' or '-', contain '..', or be a reserved Windows name such as con or
com1. A name that differs from an existing vault only by case is refused.

The password is asked twice on a terminal, or taken from SECRIO_PASSWORD.
Creating a vault does not unlock it.`,
		Example: `  secrio vault create work
  SECRIO_PASSWORD=... secrio vault create ci`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validVaultName(name); err != nil {
				return err
			}
			svc, err := d.NewService()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			entries, err := svc.ListVaults(ctx)
			if err != nil {
				return err
			}
			for _, e := range entries {
				switch {
				case e.Name == name:
					return fmt.Errorf("vault %q already exists", name)
				case strings.EqualFold(e.Name, name):
					return fmt.Errorf("vault %q already exists as %q (vault names are compared without regard to case)", name, e.Name)
				}
			}
			dir, err := d.DataDir()
			if err != nil {
				return err
			}
			path, err := core.VaultPath(dir, name)
			if err != nil {
				return err
			}
			pw, err := readPassword(d, "Password: ", true)
			if err != nil {
				return err
			}
			defer clear(pw)
			if len(pw) == 0 {
				return fmt.Errorf("password must not be empty")
			}
			if _, err := svc.CreateVault(ctx, name, path, pw); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "created vault %s\nnext: secrio unlock %s\n", name, name)
			return nil
		},
	}
}

func newVaultListCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List vaults",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := d.NewService()
			if err != nil {
				return err
			}
			entries, err := svc.ListVaults(cmd.Context())
			if err != nil {
				return err
			}
			names := make([]string, len(entries))
			for i, e := range entries {
				names[i] = e.Name
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintln(d.Out, n)
			}
			return nil
		},
	}
}
