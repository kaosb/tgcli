package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newExportCmd(flags *rootFlags) *cobra.Command {
	var outputFile string
	var fromDB bool

	cmd := &cobra.Command{
		Use:   "export <chat>",
		Short: "Export chat messages as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatArg := args[0]

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			w := os.Stdout
			if outputFile != "" {
				f, err := os.Create(outputFile)
				if err != nil {
					return fmt.Errorf("create output file: %w", err)
				}
				defer f.Close()
				w = f
			}

			if fromDB {
				// Export directly from local DB, no TG connection needed.
				return a.ExportChat(context.Background(), chatArg, w, true)
			}

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				return a.ExportChat(ctx, chatArg, w, false)
			})
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "output file (default: stdout)")
	cmd.Flags().BoolVar(&fromDB, "local", false, "export from local DB (no Telegram connection)")
	return cmd
}
