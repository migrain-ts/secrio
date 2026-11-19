package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"secrio/internal/domain"
)

func newGetCmd(d Deps, flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "get <NAME>",
		Short: "Print a secret's value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.ParseSecretName(args[0])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			u, en, err := openSecretEnv(ctx, d, flags)
			if err != nil {
				return err
			}
			defer u.Close()
			s, err := u.GetSecret(ctx, en.ID, name)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("secret not found: %s", name)
				}
				return err
			}
			defer clear(s.Value)
			if _, err := d.Out.Write(s.Value); err != nil {
				return err
			}
			if d.StdoutTTY() {
				_, err = fmt.Fprintln(d.Out)
			}
			return err
		},
	}
}
