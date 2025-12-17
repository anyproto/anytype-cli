package revoke

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/cmd/cmdutil"
	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewRevokeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an API key",
		Long:  "Revoke an API key by its Id",
		Args:  cmdutil.ExactArgs(1, "cannot revoke API key: id argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			appId := args[0]

			err := core.RevokeAPIKey(appId)
			if err != nil {
				return output.Error("Failed to revoke API key: %w", err)
			}

			output.Success("API key with Id '%s' revoked successfully", appId)
			return nil
		},
	}

	return cmd
}
