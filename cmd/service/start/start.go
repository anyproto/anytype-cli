package start

import (
	"errors"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/serviceprogram"
)

func NewStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the service",
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

			err = s.Start()
			if err != nil {
				return output.Error("Failed to start service: %w", err)
			}

			output.Success("anytype service started")
			return nil
		},
	}
}
