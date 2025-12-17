package install

import (
	"github.com/spf13/cobra"

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
			s, err := serviceprogram.GetServiceWithAddress(listenAddress)
			if err != nil {
				return output.Error("Failed to create service: %w", err)
			}

			err = s.Install()
			if err != nil {
				return output.Error("Failed to install service: %w", err)
			}

			output.Success("anytype service installed successfully")
			if listenAddress != config.DefaultAPIAddress {
				output.Info("API will listen on %s", listenAddress)
			}
			output.Print("\nTo manage the service:")
			output.Print("  Start:   anytype service start")
			output.Print("  Stop:    anytype service stop")
			output.Print("  Restart: anytype service restart")
			output.Print("  Status:  anytype service status")

			return nil
		},
	}

	cmd.Flags().StringVar(&listenAddress, "listen-address", config.DefaultAPIAddress, "API listen address in `host:port` format")

	return cmd
}
