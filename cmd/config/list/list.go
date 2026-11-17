package list

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List settings with their effective values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stored, err := config.LoadStoredConfig()
			if err != nil {
				return output.Error("Failed to read settings: %w", err)
			}

			output.Print("%-16s %-24s %s", "KEY", "VALUE", "SOURCE")
			for _, s := range config.SettingKeys() {
				value, source, warning := s.Resolve(stored.Settings)
				if warning != nil {
					output.Warning("%s: %v", s.Key, warning)
				}
				output.Print("%-16s %-24s %s", s.Key, value, cmdutil.SourceLabel(source, s.Env))
			}
			return nil
		},
	}
}
