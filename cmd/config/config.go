package config

import (
	"github.com/spf13/cobra"

	configGetCmd "github.com/anyproto/anytype-cli/cmd/config/get"
	configListCmd "github.com/anyproto/anytype-cli/cmd/config/list"
	configSetCmd "github.com/anyproto/anytype-cli/cmd/config/set"
	configUnsetCmd "github.com/anyproto/anytype-cli/cmd/config/unset"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "View and change settings",
		Long: `View and change settings saved in ~/.anytype/config.json. Settings survive logout.

Keys:
  listen-address  JSON API listen address (host:port). Change it with
                  --listen-address on 'service install', 'serve', 'auth login'
                  or 'auth create'.
  prefer-tcp      Connect to sync nodes and peers over TCP before QUIC (UDP).
                  Turn it on if sync stalls on a network that blocks or
                  throttles UDP. QUIC is still used when TCP fails.
                  Override with ANYTYPE_PREFER_TCP=true|false.`,
		Example: `  anytype config list
  anytype config set prefer-tcp true
  anytype service restart`,
	}

	cmd.AddCommand(configListCmd.NewListCmd())
	cmd.AddCommand(configGetCmd.NewGetCmd())
	cmd.AddCommand(configSetCmd.NewSetCmd())
	cmd.AddCommand(configUnsetCmd.NewUnsetCmd())

	return cmd
}
