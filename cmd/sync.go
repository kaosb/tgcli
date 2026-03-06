package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newSyncCmd(flags *rootFlags) *cobra.Command {
	var chatArg string
	var msgsPerChat int

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync message history to local DB",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				if chatArg != "" {
					fmt.Printf("Syncing %s...\n", chatArg)
					err := a.SyncChat(ctx, chatArg, func(fetched int) {
						fmt.Printf("\r  %d messages synced", fetched)
					})
					fmt.Println()
					if err != nil {
						return err
					}
				} else {
					fmt.Println("Syncing all chats...")
					err := a.SyncAllChats(ctx, msgsPerChat, func(chatName string, fetched int) {
						fmt.Printf("  %s: %d messages\n", chatName, fetched)
					})
					if err != nil {
						return err
					}
				}

				msgCount, _ := a.DB().CountMessages()
				chatCount, _ := a.DB().CountChats()
				fmt.Printf("Done. Local DB: %d messages in %d chats.\n", msgCount, chatCount)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&chatArg, "chat", "", "sync specific chat (username, phone, or ID)")
	cmd.Flags().IntVar(&msgsPerChat, "msgs-per-chat", 100, "messages per chat when syncing all")
	return cmd
}
