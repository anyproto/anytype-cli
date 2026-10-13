package set

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "set <key> <value>",
		Short:             "Save a setting",
		Args:              cmdutil.ExactArgs(2, "cannot set config: key and value arguments required"),
		ValidArgsFunction: cmdutil.SettingKeyCompletion(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := config.LookupSetting(args[0])
			if err != nil {
				return output.Error("%w", err)
			}

			var saved string
			err = config.UpdateSettingsInConfig(func(settings *config.Settings) error {
				var err error
				saved, err = s.Set(settings, args[1])
				return err
			})
			if err != nil {
				return output.Error("Failed to set %s: %w", s.Key, err)
			}

			output.Success("%s = %s", s.Key, saved)
			cmdutil.ExplainSettingChange(s)
			return nil
		},
	}
}
