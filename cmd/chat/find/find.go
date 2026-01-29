package find

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/output"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func NewFindCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find <space-id>",
		Short: "Find chat objects in a space",
		Long:  "Search for objects with chat functionality in a space and display their chat IDs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spaceId := args[0]

			var found bool
			err := core.GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
				req := &pb.RpcObjectSearchRequest{
					SpaceId: spaceId,
					Keys:    []string{"id", "name", "chatId", "hasChat", "layout"},
					Limit:   100,
				}
				resp, err := client.ObjectSearch(ctx, req)
				if err != nil {
					return fmt.Errorf("failed to search objects: %w", err)
				}
				if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
					return fmt.Errorf("search error: %s", resp.Error.Description)
				}

				output.Info("%-40s %-20s %s", "CHAT ID", "NAME", "OBJECT ID")
				output.Info("%-40s %-20s %s", "───────", "────", "─────────")

				for _, record := range resp.Records {
					chatId := pbtypes.GetString(record, "chatId")
					hasChat := pbtypes.GetBool(record, "hasChat")

					if chatId != "" || hasChat {
						found = true
						name := pbtypes.GetString(record, "name")
						objId := pbtypes.GetString(record, "id")
						if len(name) > 18 {
							name = name[:15] + "..."
						}
						if chatId == "" {
							chatId = "(no chatId set)"
						}
						output.Info("%-40s %-20s %s", chatId, name, objId)
					}
				}
				return nil
			})

			if err != nil {
				return output.Error("Failed to find chats: %w", err)
			}

			if !found {
				output.Info("No chat objects found in this space")
			}

			return nil
		},
	}

	return cmd
}
