package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newLoginCmd(flags *rootFlags) *cobra.Command {
	var phone string
	var code string
	var password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Telegram (phone + code)",
		Long: `Authenticate with your personal Telegram account.

Requires TGCLI_APP_ID and TGCLI_APP_HASH environment variables
(get them from https://my.telegram.org/apps).

You will be prompted for your phone number, verification code,
and optionally your 2FA password. Or pass them via flags.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			if err := a.Login(ctx, phone, code, password); err != nil {
				return err
			}

			fmt.Println("Logged in successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&phone, "phone", "", "phone number (e.g. +56...)")
	cmd.Flags().StringVar(&code, "code", "", "verification code")
	cmd.Flags().StringVar(&password, "password", "", "2FA password")
	return cmd
}
