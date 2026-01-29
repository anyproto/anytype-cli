package list

import (
	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
)

var (
	limit   int32
	before  string
	after   string
	reverse bool
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <chat-id>",
		Short: "List messages in a chat",
		Long:  "Retrieve and display messages from an Anytype chat object",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chatId := args[0]

			messages, err := core.GetChatMessages(chatId, limit, before, after)
			if err != nil {
				return output.Error("Failed to get messages: %w", err)
			}

			if len(messages) == 0 {
				output.Info("No messages found")
				return nil
			}

			// Print in chronological order (oldest first) unless reversed
			start, end, step := 0, len(messages), 1
			if reverse {
				start, end, step = len(messages)-1, -1, -1
			}

			for i := start; i != end; i += step {
				msg := messages[i]
				timestamp := msg.CreatedAt.Format("2006-01-02 15:04:05")
				readMark := " "
				if !msg.Read {
					readMark = "●"
				}

				output.Info("%s [%s] %s:", readMark, timestamp, msg.Creator)
				output.Info("   %s", msg.Text)

				if len(msg.Reactions) > 0 {
					reactionStr := "   Reactions:"
					for emoji, users := range msg.Reactions {
						reactionStr += " " + emoji + "(" + string(rune(len(users)+'0')) + ")"
					}
					output.Info(reactionStr)
				}

				if msg.ReplyTo != "" {
					output.Info("   ↳ Reply to: %s", msg.ReplyTo)
				}

				output.Info("   ID: %s", msg.ID)
				output.Info("")
			}

			return nil
		},
	}

	cmd.Flags().Int32VarP(&limit, "limit", "n", 20, "Maximum number of messages to retrieve")
	cmd.Flags().StringVar(&before, "before", "", "Get messages before this order ID")
	cmd.Flags().StringVar(&after, "after", "", "Get messages after this order ID")
	cmd.Flags().BoolVarP(&reverse, "reverse", "r", false, "Show newest messages first")

	return cmd
}
