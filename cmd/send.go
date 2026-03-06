package cmd

import (
	"context"
	"fmt"

	"github.com/kaosb/tgcli/internal/out"
	"github.com/spf13/cobra"
)

func newSendCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send messages",
	}
	cmd.AddCommand(newSendTextCmd(flags))
	cmd.AddCommand(newSendFileCmd(flags))
	return cmd
}

func newSendTextCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "text <chat> <message>",
		Short: "Send a text message",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatArg := args[0]
			message := args[1]

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				if err := a.SendText(ctx, chatArg, message); err != nil {
					return err
				}

				if flags.asJSON {
					return out.WriteJSON(cmd.OutOrStdout(), map[string]any{
						"sent": true,
						"to":   chatArg,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Sent to %s\n", chatArg)
				return nil
			})
		},
	}
	return cmd
}

func newSendFileCmd(flags *rootFlags) *cobra.Command {
	var caption string

	cmd := &cobra.Command{
		Use:   "file <chat> <path>",
		Short: "Send a file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatArg := args[0]
			filePath := args[1]

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				if err := a.SendFile(ctx, chatArg, filePath, caption); err != nil {
					return err
				}

				if flags.asJSON {
					return out.WriteJSON(cmd.OutOrStdout(), map[string]any{
						"sent": true,
						"to":   chatArg,
						"file": filePath,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Sent %s to %s\n", filePath, chatArg)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&caption, "caption", "", "file caption")
	return cmd
}
