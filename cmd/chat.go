package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/kaosb/tgcli/internal/out"
	"github.com/kaosb/tgcli/internal/store"
	"github.com/spf13/cobra"
)

func newChatCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Manage chats",
	}
	cmd.AddCommand(newChatLsCmd(flags))
	return cmd
}

func newChatLsCmd(flags *rootFlags) *cobra.Command {
	var chatType string
	var limit int

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List chats",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				chats, err := a.ListChats(ctx, chatType, limit)
				if err != nil {
					return err
				}

				if flags.asJSON {
					return out.WriteJSON(os.Stdout, chats)
				}

				return printChats(chats)
			})
		},
	}

	cmd.Flags().StringVar(&chatType, "type", "", "filter by type (private, group, channel)")
	cmd.Flags().IntVar(&limit, "limit", 50, "max chats to show")
	return cmd
}

func printChats(chats []store.Chat) error {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tNAME\tID")
	for _, c := range chats {
		name := c.Name
		if name == "" {
			name = c.PeerID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Kind, truncate(name, 40), c.PeerID)
	}
	return w.Flush()
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
