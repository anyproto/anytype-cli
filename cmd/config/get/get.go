package get

import (
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/output"
	"github.com/spf13/cobra"
)

func NewGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [key]",
		Short: "Get a configuration value",
		Long:  `Get a specific configuration value or all values if no key is specified`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				accountId, _ := config.GetAccountIdFromConfig()
				techSpaceId, _ := config.GetTechSpaceIdFromConfig()
				networkId, _ := config.GetNetworkIdFromConfig()

				if accountId != "" {
					output.Info("accountId: %s", accountId)
				}
				if techSpaceId != "" {
					output.Info("techSpaceId: %s", techSpaceId)
				}
				if networkId != "" {
					output.Info("networkId: %s", networkId)
				}
				return nil
			}

			key := args[0]
			switch key {
			case "accountId":
				accountId, _ := config.GetAccountIdFromConfig()
				if accountId != "" {
					output.Info(accountId)
				}
			case "techSpaceId":
				techSpaceId, _ := config.GetTechSpaceIdFromConfig()
				if techSpaceId != "" {
					output.Info(techSpaceId)
				}
			case "networkId":
				networkId, _ := config.GetNetworkIdFromConfig()
				if networkId != "" {
					output.Info(networkId)
				}
			default:
				return output.Error("unknown config key: %s", key)
			}

			return nil
		},
	}
}
