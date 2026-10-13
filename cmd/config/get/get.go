package get

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "get <key>",
		Short:             "Print a setting's effective value",
		Long:              "Print a setting's effective value, including any environment override. Run 'anytype config list' to see every key.",
		Args:              cmdutil.ExactArgs(1, "cannot get config: key argument required"),
		ValidArgsFunction: cmdutil.SettingKeyCompletion(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := config.LookupSetting(args[0])
			if err != nil {
				return output.Error("%w", err)
			}
			stored, err := config.LoadStoredConfig()
			if err != nil {
				return output.Error("Failed to read settings: %w", err)
			}

			value, _, warning := s.Resolve(stored.Settings)
			if warning != nil {
				output.Warning("%s: %v", s.Key, warning)
			}
			output.Print("%s", value)
			return nil
		},
	}
}
