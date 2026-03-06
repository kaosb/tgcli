package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaosb/tgcli/internal/app"
	"github.com/spf13/cobra"
)

// version is set via ldflags at build time.
var version = "dev"

type rootFlags struct {
	storeDir string
	asJSON   bool
	timeout  time.Duration
}

func Execute(args []string) error {
	var flags rootFlags

	rootCmd := &cobra.Command{
		Use:           "tgcli",
		Short:         "Telegram CLI for personal accounts",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	rootCmd.SetVersionTemplate("tgcli {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&flags.storeDir, "store", "", "store directory (default: ~/.tgcli)")
	rootCmd.PersistentFlags().BoolVar(&flags.asJSON, "json", false, "output JSON instead of human-readable text")
	rootCmd.PersistentFlags().DurationVar(&flags.timeout, "timeout", 5*time.Minute, "command timeout")

	rootCmd.AddCommand(newLoginCmd(&flags))
	rootCmd.AddCommand(newSendCmd(&flags))
	rootCmd.AddCommand(newChatCmd(&flags))
	rootCmd.AddCommand(newMsgCmd(&flags))
	rootCmd.AddCommand(newSyncCmd(&flags))
	rootCmd.AddCommand(newDownloadCmd(&flags))
	rootCmd.AddCommand(newExportCmd(&flags))

	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return err
	}
	return nil
}

func newApp(flags *rootFlags) (*app.App, error) {
	storeDir := flags.storeDir
	if storeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		storeDir = filepath.Join(home, ".tgcli")
	}
	storeDir, _ = filepath.Abs(storeDir)

	return app.New(app.Options{
		StoreDir: storeDir,
		JSON:     flags.asJSON,
	})
}

func withTimeout(ctx context.Context, flags *rootFlags) (context.Context, context.CancelFunc) {
	if flags.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, flags.timeout)
}
