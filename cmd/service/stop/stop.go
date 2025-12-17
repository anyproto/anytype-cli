package stop

import (
	"errors"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/serviceprogram"
)

func NewStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the service",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := serviceprogram.GetService()
			if err != nil {
				return output.Error("Failed to create service: %w", err)
			}

			_, err = s.Status()
			if err != nil && errors.Is(err, service.ErrNotInstalled) {
				output.Warning("anytype service is not installed")
				output.Info("Run 'anytype service install' to install it first")
				return nil
			}

			err = s.Stop()
			if err != nil {
				return output.Error("Failed to stop service: %w", err)
			}

			output.Success("anytype service stopped")
			return nil
		},
	}
}
