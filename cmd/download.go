package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kaosb/tgcli/internal/out"
	"github.com/spf13/cobra"
)

func newDownloadCmd(flags *rootFlags) *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "download <chat> <msg_id>",
		Short: "Download media from a message",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatArg := args[0]
			msgID, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid msg_id: %w", err)
			}

			if outputDir == "" {
				outputDir, _ = os.Getwd()
			}
			outputDir, _ = filepath.Abs(outputDir)

			a, err := newApp(flags)
			if err != nil {
				return err
			}
			defer a.Close()

			ctx, cancel := withTimeout(context.Background(), flags)
			defer cancel()

			return a.RunTG(ctx, func(ctx context.Context) error {
				path, err := a.DownloadMedia(ctx, chatArg, msgID, outputDir)
				if err != nil {
					return err
				}

				if flags.asJSON {
					return out.WriteJSON(os.Stdout, map[string]any{
						"downloaded": true,
						"path":       path,
					})
				}
				fmt.Printf("Downloaded: %s\n", path)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "output directory (default: current)")
	return cmd
}
