package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/updatecheck"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/auth"
	"github.com/anyproto/anytype-cli/cmd/config"
	"github.com/anyproto/anytype-cli/cmd/serve"
	"github.com/anyproto/anytype-cli/cmd/service"
	"github.com/anyproto/anytype-cli/cmd/shell"
	"github.com/anyproto/anytype-cli/cmd/space"
	"github.com/anyproto/anytype-cli/cmd/update"
	"github.com/anyproto/anytype-cli/cmd/version"
)

var (
	versionFlag   bool
	noUpdateCheck bool
	rootCmd       = &cobra.Command{
		Use:   "anytype <command> <subcommand> [flags]",
		Short: "Command-line interface for Anytype",
		Long:  "Command-line interface for Anytype",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if !shouldCheckUpdate(cmd) {
				return
			}
			updatecheck.Start(context.Background())
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if !shouldCheckUpdate(cmd) {
				return
			}
			if msg, ok := updatecheck.Hint(core.GetVersion()); ok {
				output.Warning(msg)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if versionFlag {
				output.Print(core.GetVersionBrief())
				return
			}
			_ = cmd.Help()
		},
	}
)

func Execute() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Show version information")
	rootCmd.Flags().BoolP("help", "h", false, "Show help for command")
	rootCmd.PersistentFlags().BoolVar(&noUpdateCheck, "no-update-check", false, "Disable the background check for new CLI releases")

	rootCmd.AddCommand(
		auth.NewAuthCmd(),
		config.NewConfigCmd(),
		serve.NewServeCmd(),
		service.NewServiceCmd(),
		shell.NewShellCmd(rootCmd),
		space.NewSpaceCmd(),
		update.NewUpdateCmd(),
		version.NewVersionCmd(),
	)

	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

func shouldCheckUpdate(cmd *cobra.Command) bool {
	if noUpdateCheck {
		return false
	}
	if !isTerminal(os.Stderr) {
		return false
	}
	if !strings.HasPrefix(core.GetVersion(), "v") {
		return false
	}
	switch cmd.Name() {
	case "serve", "shell", "update":
		return false
	}
	return true
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
