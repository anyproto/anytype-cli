package delete

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <chat-id> <message-id>",
		Short: "Delete a message",
		Long:  "Delete a message from a chat",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatId := args[0]
			messageId := args[1]

			err := core.DeleteChatMessage(chatId, messageId)
			if err != nil {
				return output.Error("Failed to delete message: %w", err)
			}

			output.Info("Message deleted successfully")
			return nil
		},
	}

	return cmd
}
