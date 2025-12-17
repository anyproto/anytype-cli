package status

import (
	"errors"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/serviceprogram"
)

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := serviceprogram.GetService()
			if err != nil {
				return output.Error("Failed to create service: %w", err)
			}

			status, err := s.Status()
			if err != nil {
				if errors.Is(err, service.ErrNotInstalled) {
					output.Info("anytype service is not installed")
					output.Info("Run 'anytype service install' to install it")
					return nil
				}
				return output.Error("Failed to get service status: %w", err)
			}

			switch status {
			case service.StatusRunning:
				output.Success("anytype service is running")
			case service.StatusStopped:
				output.Info("anytype service is stopped")
				output.Info("Run 'anytype service start' to start it")
			default:
				output.Info("anytype service status: %v", status)
			}

			return nil
		},
	}
}
