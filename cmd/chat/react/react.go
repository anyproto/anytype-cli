package react

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
)

func NewReactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "react <chat-id> <message-id> <emoji>",
		Short: "React to a message",
		Long:  "Add or remove a reaction (emoji) from a message. Running twice toggles the reaction off.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatId := args[0]
			messageId := args[1]
			emoji := args[2]

			added, err := core.ToggleChatReaction(chatId, messageId, emoji)
			if err != nil {
				return output.Error("Failed to toggle reaction: %w", err)
			}

			if added {
				output.Info("Reaction %s added", emoji)
			} else {
				output.Info("Reaction %s removed", emoji)
			}
			return nil
		},
	}

	return cmd
}
