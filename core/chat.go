package core

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// ChatMessage represents a chat message for display
type ChatMessage struct {
	ID        string
	OrderID   string
	Creator   string
	Text      string
	CreatedAt time.Time
	Reactions map[string][]string // emoji -> user IDs
	ReplyTo   string
	Read      bool
}

// SendChatMessage sends a message to a chat object
func SendChatMessage(chatObjectId string, text string, replyToMsgId string) (string, error) {
	var msgId string
	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		msg := &model.ChatMessage{
			Message: &model.ChatMessageMessageContent{
				Text: text,
			},
		}
		if replyToMsgId != "" {
			msg.ReplyToMessageId = replyToMsgId
		}

		req := &pb.RpcChatAddMessageRequest{
			ChatObjectId: chatObjectId,
			Message:      msg,
		}
		resp, err := client.ChatAddMessage(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatAddMessageResponseError_NULL {
			return fmt.Errorf("send message error: %s", resp.Error.Description)
		}
		msgId = resp.MessageId
		return nil
	})
	return msgId, err
}

// GetChatMessages retrieves messages from a chat object
func GetChatMessages(chatObjectId string, limit int32, beforeOrderId string, afterOrderId string) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcChatGetMessagesRequest{
			ChatObjectId:  chatObjectId,
			Limit:         limit,
			BeforeOrderId: beforeOrderId,
			AfterOrderId:  afterOrderId,
		}
		resp, err := client.ChatGetMessages(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to get messages: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL {
			return fmt.Errorf("get messages error: %s", resp.Error.Description)
		}

		for _, m := range resp.Messages {
			msg := ChatMessage{
				ID:        m.Id,
				OrderID:   m.OrderId,
				Creator:   m.Creator,
				CreatedAt: time.Unix(m.CreatedAt, 0),
				ReplyTo:   m.ReplyToMessageId,
				Read:      m.Read,
			}
			if m.Message != nil {
				msg.Text = m.Message.Text
			}
			if m.Reactions != nil {
				msg.Reactions = make(map[string][]string)
				for emoji, identList := range m.Reactions.Reactions {
					msg.Reactions[emoji] = identList.Ids
				}
			}
			messages = append(messages, msg)
		}
		return nil
	})
	return messages, err
}

// EditChatMessage edits an existing message
func EditChatMessage(chatObjectId string, messageId string, newText string) error {
	return GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcChatEditMessageContentRequest{
			ChatObjectId: chatObjectId,
			MessageId:    messageId,
			EditedMessage: &model.ChatMessage{
				Message: &model.ChatMessageMessageContent{
					Text: newText,
				},
			},
		}
		resp, err := client.ChatEditMessageContent(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to edit message: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatEditMessageContentResponseError_NULL {
			return fmt.Errorf("edit message error: %s", resp.Error.Description)
		}
		return nil
	})
}

// DeleteChatMessage deletes a message from a chat
func DeleteChatMessage(chatObjectId string, messageId string) error {
	return GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcChatDeleteMessageRequest{
			ChatObjectId: chatObjectId,
			MessageId:    messageId,
		}
		resp, err := client.ChatDeleteMessage(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to delete message: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatDeleteMessageResponseError_NULL {
			return fmt.Errorf("delete message error: %s", resp.Error.Description)
		}
		return nil
	})
}

// ToggleChatReaction adds or removes a reaction from a message
func ToggleChatReaction(chatObjectId string, messageId string, emoji string) (bool, error) {
	var added bool
	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcChatToggleMessageReactionRequest{
			ChatObjectId: chatObjectId,
			MessageId:    messageId,
			Emoji:        emoji,
		}
		resp, err := client.ChatToggleMessageReaction(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to toggle reaction: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatToggleMessageReactionResponseError_NULL {
			return fmt.Errorf("toggle reaction error: %s", resp.Error.Description)
		}
		added = resp.Added
		return nil
	})
	return added, err
}

// SubscribeToChat subscribes to a chat and returns the latest messages
func SubscribeToChat(chatObjectId string, limit int32) ([]ChatMessage, error) {
	var messages []ChatMessage
	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcChatSubscribeLastMessagesRequest{
			ChatObjectId: chatObjectId,
			Limit:        limit,
			SubId:        "cli-sub",
		}
		resp, err := client.ChatSubscribeLastMessages(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to subscribe to chat: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatSubscribeLastMessagesResponseError_NULL {
			return fmt.Errorf("subscribe error: %s", resp.Error.Description)
		}

		for _, m := range resp.Messages {
			msg := ChatMessage{
				ID:        m.Id,
				OrderID:   m.OrderId,
				Creator:   m.Creator,
				CreatedAt: time.Unix(m.CreatedAt, 0),
				ReplyTo:   m.ReplyToMessageId,
				Read:      m.Read,
			}
			if m.Message != nil {
				msg.Text = m.Message.Text
			}
			if m.Reactions != nil {
				msg.Reactions = make(map[string][]string)
				for emoji, identList := range m.Reactions.Reactions {
					msg.Reactions[emoji] = identList.Ids
				}
			}
			messages = append(messages, msg)
		}
		return nil
	})
	return messages, err
}

// FindChatObjects searches for chat-type objects in a space
func FindChatObjects(spaceId string) ([]string, error) {
	var chatIds []string
	err := GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: "type",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("ot-chat"),
				},
			},
			Limit: 100,
		}
		resp, err := client.ObjectSearch(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to search objects: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return fmt.Errorf("search error: %s", resp.Error.Description)
		}

		for _, record := range resp.Records {
			if id := pbtypes.GetString(record, "id"); id != "" {
				chatIds = append(chatIds, id)
			}
		}
		return nil
	})
	return chatIds, err
}

// MarkChatMessagesRead marks messages as read up to a certain point
func MarkChatMessagesRead(chatObjectId string, afterOrderId string, beforeOrderId string) error {
	return GRPCCall(func(ctx context.Context, client service.ClientCommandsClient) error {
		req := &pb.RpcChatReadMessagesRequest{
			ChatObjectId:  chatObjectId,
			AfterOrderId:  afterOrderId,
			BeforeOrderId: beforeOrderId,
		}
		resp, err := client.ChatReadMessages(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to mark messages read: %w", err)
		}
		if resp.Error != nil && resp.Error.Code != pb.RpcChatReadMessagesResponseError_NULL {
			return fmt.Errorf("mark read error: %s", resp.Error.Description)
		}
		return nil
	})
}
