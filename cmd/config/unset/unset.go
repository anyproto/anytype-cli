package unset

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "unset <key>",
		Short:             "Return a setting to its default",
		Args:              cmdutil.ExactArgs(1, "cannot unset config: key argument required"),
		ValidArgsFunction: cmdutil.SettingKeyCompletion(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := config.LookupSetting(args[0])
			if err != nil {
				return output.Error("%w", err)
			}

			var saved string
			err = config.UpdateSettingsInConfig(func(settings *config.Settings) error {
				var err error
				saved, err = s.Unset(settings)
				return err
			})
			if err != nil {
				return output.Error("Failed to unset %s: %w", s.Key, err)
			}

			output.Success("%s = %s (default)", s.Key, saved)
			cmdutil.ExplainSettingChange(s)
			return nil
		},
	}
}
