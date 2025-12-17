package service

import (
	"github.com/spf13/cobra"

	serviceInstallCmd "github.com/anyproto/anytype-cli/cmd/service/install"
	serviceRestartCmd "github.com/anyproto/anytype-cli/cmd/service/restart"
	serviceStartCmd "github.com/anyproto/anytype-cli/cmd/service/start"
	serviceStatusCmd "github.com/anyproto/anytype-cli/cmd/service/status"
	serviceStopCmd "github.com/anyproto/anytype-cli/cmd/service/stop"
	serviceUninstallCmd "github.com/anyproto/anytype-cli/cmd/service/uninstall"
)

func NewServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service <command>",
		Short: "Manage anytype as a user service",
		Long:  "Install, uninstall, start, stop, and check status of anytype running as a user service.",
	}

	cmd.AddCommand(serviceInstallCmd.NewInstallCmd())
	cmd.AddCommand(serviceUninstallCmd.NewUninstallCmd())
	cmd.AddCommand(serviceStartCmd.NewStartCmd())
	cmd.AddCommand(serviceStopCmd.NewStopCmd())
	cmd.AddCommand(serviceRestartCmd.NewRestartCmd())
	cmd.AddCommand(serviceStatusCmd.NewStatusCmd())

	return cmd
}
