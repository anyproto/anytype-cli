package cmd

import (
	"fmt"
	"os"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/auth"
	"github.com/anyproto/anytype-cli/cmd/chat"
	"github.com/anyproto/anytype-cli/cmd/config"
	"github.com/anyproto/anytype-cli/cmd/serve"
	"github.com/anyproto/anytype-cli/cmd/service"
	"github.com/anyproto/anytype-cli/cmd/shell"
	"github.com/anyproto/anytype-cli/cmd/space"
	"github.com/anyproto/anytype-cli/cmd/update"
	"github.com/anyproto/anytype-cli/cmd/version"
)

var (
	versionFlag bool
	rootCmd     = &cobra.Command{
		Use:   "anytype <command> <subcommand> [flags]",
		Short: "Command-line interface for Anytype",
		Long:  "Command-line interface for Anytype",
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

	rootCmd.AddCommand(
		auth.NewAuthCmd(),
		chat.NewChatCmd(),
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
