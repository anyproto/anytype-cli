package set

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long:  `Set a specific configuration value`,
		Args:  cmdutil.ExactArgs(2, "cannot set config: key and value arguments required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			switch key {
			case "accountId":
				if err := config.SetAccountIdToConfig(value); err != nil {
					return output.Error("Failed to set account Id: %w", err)
				}
			case "techSpaceId":
				if err := config.SetTechSpaceIdToConfig(value); err != nil {
					return output.Error("Failed to set tech space Id: %w", err)
				}
			case "networkId":
				if err := config.SetNetworkIdToConfig(value); err != nil {
					return output.Error("Failed to set network Id: %w", err)
				}
			default:
				return output.Error("unknown config key: %s", key)
			}

			output.Success("Set %s = %s", key, value)
			return nil
		},
	}
}
