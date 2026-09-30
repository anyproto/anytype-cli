package install

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/serviceprogram"
)

func NewInstallCmd() *cobra.Command {
	var listenAddress string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install as a user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiAddr, explicit := cmdutil.APIListenAddr(cmd, listenAddress)

			s, err := serviceprogram.GetServiceWithAddress(apiAddr)
			if err != nil {
				return output.Error("Failed to create service: %w", err)
			}

			err = s.Install()
			if err != nil {
				return output.Error("Failed to install service: %w", err)
			}

			if explicit {
				if err := config.SetApiListenAddrToConfig(apiAddr); err != nil {
					output.Warning("Failed to remember the JSON API address: %v", err)
				}
			}

			output.Success("anytype service installed successfully")
			output.Banner("JSON API: "+config.APIURL(apiAddr), "starts when an account is logged in")
			output.Print("\nTo manage the service:")
			output.Print("  Start:   anytype service start")
			output.Print("  Stop:    anytype service stop")
			output.Print("  Restart: anytype service restart")
			output.Print("  Status:  anytype service status")

			return nil
		},
	}

	cmdutil.AddListenAddressFlag(cmd, &listenAddress)

	return cmd
}
