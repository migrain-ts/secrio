package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newUnlockCmd(d Deps, flags *Flags) *cobra.Command {
	var ttl, idle time.Duration
	cmd := &cobra.Command{
		Use:   "unlock [vault]",
		Short: "Unlock a vault for a limited time",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if ttl <= 0 {
				return UsageErr("--ttl must be positive")
			}
			if idle < 0 {
				return UsageErr("--idle must not be negative")
			}
			svc, err := d.NewService()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			entry, err := selectVault(ctx, d, svc, flags, argOrEmpty(args))
			if err != nil {
				return err
			}
			pw, err := readPassword(d, "Password: ", false)
			if err != nil {
				return err
			}
			defer clear(pw)
			if err := svc.Unlock(ctx, entry.ID, pw, ttl, idle); err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "unlocked %s (expires in %s)\n", entry.Name, ttl.String())
			return nil
		},
	}
	cmd.Flags().DurationVar(&ttl, "ttl", 12*time.Hour, "maximum session lifetime")
	cmd.Flags().DurationVar(&idle, "idle", time.Hour, "idle timeout (0 disables)")
	return cmd
}

func newLockCmd(d Deps, flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "lock [vault]",
		Short: "Lock a vault (end its session)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := d.NewService()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			entry, err := selectVault(ctx, d, svc, flags, argOrEmpty(args))
			if err != nil {
				return err
			}
			if err := svc.Lock(ctx, entry.ID); err != nil {
				return fmt.Errorf("could not confirm the lock of %q, it may still be unlocked: %v", entry.Name, err)
			}
			fmt.Fprintf(d.Out, "locked %s\n", entry.Name)
			return nil
		},
	}
}

func argOrEmpty(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
