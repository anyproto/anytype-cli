package send

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
)

var replyTo string

func NewSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <chat-id> <message>",
		Short: "Send a message to a chat",
		Long:  "Send a text message to an Anytype chat object",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatId := args[0]
			message := args[1]

			msgId, err := core.SendChatMessage(chatId, message, replyTo)
			if err != nil {
				return output.Error("Failed to send message: %w", err)
			}

			output.Info("Message sent successfully")
			output.Info("Message ID: %s", msgId)
			return nil
		},
	}

	cmd.Flags().StringVar(&replyTo, "reply-to", "", "Message ID to reply to")

	return cmd
}
