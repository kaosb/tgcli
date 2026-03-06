package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/kaosb/tgcli/internal/out"
	"github.com/kaosb/tgcli/internal/store"
	"github.com/spf13/cobra"
)

func newMsgCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "msg",
		Short: "Read and search messages",
	}
	cmd.AddCommand(newMsgLsCmd(flags))
	cmd.AddCommand(newMsgContextCmd(flags))
	cmd.AddCommand(newMsgSearchCmd(flags))
	return cmd
}

func newMsgLsCmd(flags *rootFlags) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "ls <chat>",
		Short: "List messages from a chat",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatArg := args[0]

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				messages, err := a.ListMessages(ctx, chatArg, limit)
				if err != nil {
					return err
				}

				if flags.asJSON {
					return out.WriteJSON(os.Stdout, messages)
				}

				return printMessages(messages)
			})
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "max messages")
	return cmd
}

func newMsgContextCmd(flags *rootFlags) *cobra.Command {
	var before, after int

	cmd := &cobra.Command{
		Use:   "context <chat> <msg_id>",
		Short: "Show messages around a specific message",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatArg := args[0]
			msgID, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid msg_id: %w", err)
			}

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				messages, err := a.MessageContext(ctx, chatArg, msgID, before, after)
				if err != nil {
					return err
				}

				if flags.asJSON {
					return out.WriteJSON(os.Stdout, messages)
				}

				return printMessages(messages)
			})
		},
	}

	cmd.Flags().IntVar(&before, "before", 5, "messages before")
	cmd.Flags().IntVar(&after, "after", 5, "messages after")
	return cmd
}

func newMsgSearchCmd(flags *rootFlags) *cobra.Command {
	var chatFilter string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search messages (full-text)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			messages, err := a.DB().SearchMessages(store.SearchMessagesParams{
				Query:   query,
				ChatID:  chatFilter,
				Limit:   limit,
			})
			if err != nil {
				return err
			}

			if flags.asJSON {
				return out.WriteJSON(os.Stdout, messages)
			}

			return printMessages(messages)
		},
	}

	cmd.Flags().StringVar(&chatFilter, "chat", "", "filter by chat")
	cmd.Flags().IntVar(&limit, "limit", 20, "max results")
	return cmd
}

func printMessages(messages []store.Message) error {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	for _, m := range messages {
		sender := m.SenderName
		if sender == "" {
			sender = m.SenderID
		}
		if m.FromMe {
			sender = "me"
		}
		ts := m.Timestamp.Local().Format(time.DateTime)
		text := m.Text
		if text == "" && m.MediaType != "" {
			text = fmt.Sprintf("[%s]", m.MediaType)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", ts, sender, truncate(text, 80))
	}
	return w.Flush()
}
