package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/kaosb/tgcli/internal/app"
	"github.com/spf13/cobra"
)

var negativeID = regexp.MustCompile(`^-\d+$`)

// version is set via ldflags at build time.
var version = "dev"

type rootFlags struct {
	storeDir string
	asJSON   bool
	timeout  time.Duration
}

func Execute(args []string) error {
	// Insert "--" before negative numeric IDs so Cobra doesn't treat them as flags.
	args = fixNegativeIDs(args)

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
	rootCmd.AddCommand(newLogoutCmd(&flags))
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

// fixNegativeIDs inserts "--" before a negative numeric ID so Cobra doesn't
// treat it as a flag. Flags after the ID are moved before "--".
// Example: ["msg", "ls", "-427317440", "--limit", "3"]
//       -> ["msg", "ls", "--limit", "3", "--", "-427317440"]
// Example: ["send", "text", "-427317440", "hello"]
//       -> ["send", "text", "--", "-427317440", "hello"]
func fixNegativeIDs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if negativeID.MatchString(arg) {
			before := args[:i]
			rest := args[i:] // starts with the negative ID

			// Separate flags (--key val) from positional args after the ID.
			var flags, positional []string
			for j := 1; j < len(rest); j++ {
				if len(rest[j]) > 1 && rest[j][0] == '-' && rest[j][1] == '-' {
					// --flag value
					flags = append(flags, rest[j])
					if j+1 < len(rest) {
						flags = append(flags, rest[j+1])
						j++
					}
				} else {
					positional = append(positional, rest[j])
				}
			}

			result := make([]string, 0, len(args)+1)
			result = append(result, before...)
			result = append(result, flags...)
			result = append(result, "--")
			result = append(result, rest[0]) // the negative ID
			result = append(result, positional...)
			return result
		}
	}
	return args
}

func withTimeout(ctx context.Context, flags *rootFlags) (context.Context, context.CancelFunc) {
	if flags.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, flags.timeout)
}
