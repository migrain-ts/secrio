package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"secrio/internal/app"
)

// openContextVault loads the context and opens its vault (caller closes).
func openContextVault(ctx context.Context, d Deps, flags *Flags) (*app.Unlocked, error) {
	c, err := LoadContext(d, *flags)
	if err != nil {
		return nil, err
	}
	return openVaultFromContext(ctx, d, c)
}

func openVaultFromContext(ctx context.Context, d Deps, c Context) (*app.Unlocked, error) {
	name, err := c.NeedVault()
	if err != nil {
		return nil, err
	}
	svc, err := d.NewService()
	if err != nil {
		return nil, err
	}
	entry, err := resolveVault(ctx, svc, name)
	if err != nil {
		return nil, err
	}
	return openVault(ctx, d, svc, entry)
}

func newListCmd(d Deps, flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List projects, environments or secret names depending on the context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := LoadContext(d, *flags)
			if err != nil {
				return err
			}
			if c.Env != "" && c.Project == "" {
				return UsageErr("--env needs --project (or SECRIO_PROJECT)")
			}
			ctx := cmd.Context()
			u, err := openVaultFromContext(ctx, d, c)
			if err != nil {
				return err
			}
			defer u.Close()

			var names []string
			switch {
			case c.Project == "":
				ps, err := u.ListProjects(ctx)
				if err != nil {
					return err
				}
				for _, p := range ps {
					names = append(names, p.Name)
				}
			default:
				p, err := resolveProject(ctx, u, c.Project)
				if err != nil {
					return err
				}
				if c.Env == "" {
					es, err := u.ListEnvironments(ctx, p.ID)
					if err != nil {
						return err
					}
					for _, e := range es {
						names = append(names, e.Name)
					}
					break
				}
				en, err := resolveEnv(ctx, u, p.ID, c.Env)
				if err != nil {
					return err
				}
				ss, err := u.ListSecrets(ctx, en.ID)
				if err != nil {
					return err
				}
				for _, s := range ss {
					names = append(names, string(s.Name))
					clear(s.Value)
				}
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintln(d.Out, n)
			}
			return nil
		},
	}
}
