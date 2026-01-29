package edit

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <chat-id> <message-id> <new-text>",
		Short: "Edit a message",
		Long:  "Edit the content of an existing chat message",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatId := args[0]
			messageId := args[1]
			newText := args[2]

			err := core.EditChatMessage(chatId, messageId, newText)
			if err != nil {
				return output.Error("Failed to edit message: %w", err)
			}

			output.Info("Message edited successfully")
			return nil
		},
	}

	return cmd
}
