package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out and remove local session",
		Long:  `Terminates the current Telegram session and removes the local session file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			if err := a.Logout(ctx); err != nil {
				return err
			}

			fmt.Println("Logged out successfully.")
			return nil
		},
	}
}
