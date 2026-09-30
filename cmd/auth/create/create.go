package create

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

// NewCreateCmd creates the auth create command
func NewCreateCmd() *cobra.Command {
	var rootPath string
	var listenAddress string
	var networkConfigPath string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new bot account",
		Long:  "Create a new Anytype bot account with a generated account key. The account key is your credential for bot authentication. Use --network-config for self-hosted networks.",
		Args:  cmdutil.ExactArgs(1, "cannot create account: name argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			apiAddr, explicit := cmdutil.APIListenAddr(cmd, listenAddress)
			accountKey, accountId, savedToKeyring, err := core.CreateWallet(name, rootPath, apiAddr, networkConfigPath)
			if err != nil {
				return output.Error("Failed to create account: %w", err)
			}
			if explicit {
				if err := config.SetApiListenAddrToConfig(apiAddr); err != nil {
					output.Warning("Failed to remember the JSON API address: %v", err)
				}
			}

			output.Success("Bot account created successfully!")

			output.Warning("IMPORTANT: Save your account key in a secure location. This is the ONLY way to authenticate your bot account.")

			output.Print("")
			keyLen := len(accountKey)
			boxWidth := keyLen + 4
			if boxWidth < 24 {
				boxWidth = 24
			}

			topBorder := "╔" + strings.Repeat("═", boxWidth) + "╗"
			midBorder := "╠" + strings.Repeat("═", boxWidth) + "╣"
			botBorder := "╚" + strings.Repeat("═", boxWidth) + "╝"

			title := "BOT ACCOUNT KEY"
			titlePadding := (boxWidth - len(title)) / 2
			titleLine := "║" + strings.Repeat(" ", titlePadding) + title + strings.Repeat(" ", boxWidth-titlePadding-len(title)) + "║"

			keyLine := fmt.Sprintf("║  %s  ║", accountKey)

			output.Print(topBorder)
			output.Print(titleLine)
			output.Print(midBorder)
			output.Print(keyLine)
			output.Print(botBorder)

			output.Print("")
			output.Print("📋 Bot Account Details:")
			output.Print("   Name: %s", name)
			output.Print("   Account Id: %s", accountId)

			output.Print("")
			output.Success("You are now logged in to your new bot account.")
			if savedToKeyring {
				output.Success("Account key saved to keychain.")
			} else {
				output.Success("Account key saved to config file.")
			}
			output.Banner("JSON API listening on " + config.APIURL(apiAddr))

			return nil
		},
	}

	cmd.Flags().StringVar(&rootPath, "root-path", "", "Root path for account data")
	cmdutil.AddListenAddressFlag(cmd, &listenAddress)
	cmd.Flags().StringVar(&networkConfigPath, "network-config", "", "Path to custom network configuration YAML (for self-hosted)")

	return cmd
}
