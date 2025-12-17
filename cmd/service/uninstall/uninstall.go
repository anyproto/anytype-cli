package uninstall

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-cli/core/serviceprogram"
)

func NewUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := serviceprogram.GetService()
			if err != nil {
				return output.Error("Failed to create service: %w", err)
			}

			err = s.Uninstall()
			if err != nil {
				return output.Error("Failed to uninstall service: %w", err)
			}

			output.Success("anytype service uninstalled successfully")
			return nil
		},
	}
}
