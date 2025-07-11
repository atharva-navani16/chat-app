// internal/chat/service.go
package chat

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type ChatService struct {
	db     *sql.DB
	redis  *redis.Client
	config *config.Config
	wsHub  *WSHub
}

func NewChatService(db *sql.DB, redis *redis.Client, config *config.Config, wsHub *WSHub) *ChatService {
	return &ChatService{
		db:     db,
		redis:  redis,
		config: config,
		wsHub:  wsHub,
	}
}

// CreatePrivateChat creates a 1-on-1 chat between two users
func (s *ChatService) CreatePrivateChat(userID uuid.UUID, req *CreatePrivateChatRequest) (*ChatResponse, error) {
	// Get target user ID (by ID or username)
	var targetUserID uuid.UUID
	if req.UserID != uuid.Nil {
		targetUserID = req.UserID
	} else if req.Username != "" {
		var err error
		targetUserID, err = s.getUserIDByUsername(req.Username)
		if err != nil {
			return nil, fmt.Errorf("user not found: %v", err)
		}
	} else {
		return nil, errors.New("either user_id or username required")
	}

	// Check if chat already exists
	existingChat, err := s.findPrivateChat(userID, targetUserID)
	if err == nil {
		return s.getChatResponse(existingChat.ID, userID)
	}

	// Create new private chat
	chatID := uuid.New()
	now := time.Now()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert chat
	chatQuery := `
		INSERT INTO chats (id, type, creator_id, is_active, created_at, updated_at)
		VALUES ($1, 'private', $2, true, $3, $4)`

	_, err = tx.Exec(chatQuery, chatID, userID, now, now)
	if err != nil {
		return nil, err
	}

	// Add both users as members
	memberQuery := `
		INSERT INTO chat_members (chat_id, user_id, role, status, joined_at)
		VALUES ($1, $2, $3, 'active', $4)`

	_, err = tx.Exec(memberQuery, chatID, userID, "creator", now)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(memberQuery, chatID, targetUserID, "member", now)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return s.getChatResponse(chatID, userID)
}

// CreateGroupChat creates a group chat with multiple users
func (s *ChatService) CreateGroupChat(userID uuid.UUID, req *CreateGroupChatRequest) (*ChatResponse, error) {
	if len(req.MemberIDs) > 200 {
		return nil, errors.New("maximum 200 members allowed")
	}

	chatID := uuid.New()
	now := time.Now()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert chat
	chatQuery := `
		INSERT INTO chats (id, type, title, description, creator_id, is_active, created_at, updated_at)
		VALUES ($1, 'group', $2, $3, $4, true, $5, $6)`

	_, err = tx.Exec(chatQuery, chatID, req.Title, req.Description, userID, now, now)
	if err != nil {
		return nil, err
	}

	// Add creator as admin
	memberQuery := `
		INSERT INTO chat_members (chat_id, user_id, role, status, joined_at, invited_by)
		VALUES ($1, $2, $3, 'active', $4, $5)`

	_, err = tx.Exec(memberQuery, chatID, userID, "creator", now, nil)
	if err != nil {
		return nil, err
	}

	// Add other members
	for _, memberID := range req.MemberIDs {
		if memberID == userID {
			continue // Skip creator
		}
		_, err = tx.Exec(memberQuery, chatID, memberID, "member", now, userID)
		if err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return s.getChatResponse(chatID, userID)
}

// SendMessage sends a message to a chat
func (s *ChatService) SendMessage(userID uuid.UUID, req *SendMessageRequest) (*Message, error) {
	// Verify user can send messages to this chat
	canSend, err := s.canUserSendMessage(userID, req.ChatID)
	if err != nil {
		return nil, err
	}
	if !canSend {
		return nil, errors.New("permission denied")
	}

	// Create message
	messageID := uuid.New()
	now := time.Now()
	messageType := req.MessageType
	if messageType == "" {
		messageType = "text"
	}

	// For now, store content as plain text (encryption can be added later)
	query := `
		INSERT INTO messages (
			id, chat_id, sender_id, message_type, content,
			reply_to_message_id, file_id, is_edited, is_deleted, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, false, false, $8)
		RETURNING id, created_at`

	var createdMessage Message
	err = s.db.QueryRow(
		query,
		messageID, req.ChatID, userID, messageType, req.Content,
		req.ReplyToMessageID, req.FileID, now,
	).Scan(&createdMessage.ID, &createdMessage.CreatedAt)

	if err != nil {
		return nil, err
	}

	// Build complete message response
	message := &Message{
		ID:               messageID,
		ChatID:           req.ChatID,
		SenderID:         userID,
		MessageType:      messageType,
		Content:          req.Content,
		ReplyToMessageID: req.ReplyToMessageID,
		FileID:           req.FileID,
		IsEdited:         false,
		IsDeleted:        false,
		CreatedAt:        now,
	}

	// Get sender info
	senderInfo, err := s.getUserInfo(userID)
	if err == nil {
		message.SenderUsername = senderInfo.Username
		message.SenderName = fmt.Sprintf("%s %s", senderInfo.FirstName, senderInfo.LastName)
	}

	// Update chat's updated_at
	s.updateChatTimestamp(req.ChatID)

	return message, nil
}

// GetMessages retrieves messages from a chat with pagination
func (s *ChatService) GetMessages(userID uuid.UUID, req *GetMessagesRequest) (*MessagesResponse, error) {
	// Verify user has access to this chat
	isMember, err := s.isUserChatMember(userID, req.ChatID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errors.New("access denied")
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Build query
	query := `
		SELECT m.id, m.chat_id, m.sender_id, m.message_type, m.content,
		       m.reply_to_message_id, m.file_id, m.is_edited, m.is_deleted, m.created_at, m.edited_at,
		       u.username, u.first_name, u.last_name
		FROM messages m
		JOIN users u ON m.sender_id = u.id
		WHERE m.chat_id = $1 AND m.is_deleted = false
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := s.db.Query(query, req.ChatID, limit+1, offset) // +1 to check if there are more
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var editedAt sql.NullTime
		var firstName, lastName string

		err := rows.Scan(
			&m.ID, &m.ChatID, &m.SenderID, &m.MessageType, &m.Content,
			&m.ReplyToMessageID, &m.FileID, &m.IsEdited, &m.IsDeleted, &m.CreatedAt, &editedAt,
			&m.SenderUsername, &firstName, &lastName,
		)
		if err != nil {
			return nil, err
		}

		m.SenderName = fmt.Sprintf("%s %s", firstName, lastName)

		if editedAt.Valid {
			m.EditedAt = &editedAt.Time
		}

		messages = append(messages, m)
	}

	// Check if there are more messages
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit] // Remove the extra message
	}

	// Get total count
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM messages WHERE chat_id = $1 AND is_deleted = false`
	s.db.QueryRow(countQuery, req.ChatID).Scan(&totalCount)

	return &MessagesResponse{
		Messages:   messages,
		HasMore:    hasMore,
		TotalCount: totalCount,
		NextOffset: offset + limit,
	}, nil
}

// GetUserChats retrieves all chats for a user
func (s *ChatService) GetUserChats(userID uuid.UUID) (*ChatListResponse, error) {
	query := `
		SELECT c.id, c.type, c.title, c.description, c.creator_id, c.is_active, c.created_at, c.updated_at,
		       cm.role, COUNT(DISTINCT cm2.user_id) as member_count
		FROM chats c
		JOIN chat_members cm ON c.id = cm.chat_id
		LEFT JOIN chat_members cm2 ON c.id = cm2.chat_id AND cm2.status = 'active'
		WHERE cm.user_id = $1 AND cm.status = 'active' AND c.is_active = true
		GROUP BY c.id, c.type, c.title, c.description, c.creator_id, c.is_active, c.created_at, c.updated_at, cm.role
		ORDER BY c.updated_at DESC`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var chat Chat
		var userRole string
		var description sql.NullString
		var title sql.NullString

		err := rows.Scan(
			&chat.ID, &chat.Type, &title, &description, &chat.CreatorID, &chat.IsActive,
			&chat.CreatedAt, &chat.UpdatedAt, &userRole, &chat.MemberCount,
		)
		if err != nil {
			return nil, err
		}

		if title.Valid {
			chat.Title = title.String
		}
		if description.Valid {
			chat.Description = description.String
		}

		// Get last message
		lastMessage, _ := s.getLastMessage(chat.ID)
		chat.LastMessage = lastMessage

		// Get unread count for this user
		chat.UnreadCount = s.getUnreadCount(userID, chat.ID)

		chats = append(chats, chat)
	}

	return &ChatListResponse{
		Chats:      chats,
		TotalCount: len(chats),
	}, nil
}

// Helper functions

func (s *ChatService) findPrivateChat(userID1, userID2 uuid.UUID) (*Chat, error) {
	query := `
		SELECT c.id, c.type, c.creator_id, c.is_active, c.created_at, c.updated_at
		FROM chats c
		JOIN chat_members cm1 ON c.id = cm1.chat_id
		JOIN chat_members cm2 ON c.id = cm2.chat_id
		WHERE c.type = 'private' 
		AND cm1.user_id = $1 AND cm1.status = 'active'
		AND cm2.user_id = $2 AND cm2.status = 'active'
		LIMIT 1`

	var chat Chat
	err := s.db.QueryRow(query, userID1, userID2).Scan(
		&chat.ID, &chat.Type, &chat.CreatorID, &chat.IsActive, &chat.CreatedAt, &chat.UpdatedAt,
	)
	return &chat, err
}

func (s *ChatService) getChatResponse(chatID uuid.UUID, userID uuid.UUID) (*ChatResponse, error) {
	// Get chat details
	query := `
		SELECT c.id, c.type, c.title, c.description, c.creator_id, c.is_active, c.created_at, c.updated_at,
		       cm.role
		FROM chats c
		JOIN chat_members cm ON c.id = cm.chat_id
		WHERE c.id = $1 AND cm.user_id = $2 AND cm.status = 'active'`

	var chat Chat
	var userRole string
	var title, description sql.NullString

	err := s.db.QueryRow(query, chatID, userID).Scan(
		&chat.ID, &chat.Type, &title, &description, &chat.CreatorID, &chat.IsActive,
		&chat.CreatedAt, &chat.UpdatedAt, &userRole,
	)
	if err != nil {
		return nil, err
	}

	if title.Valid {
		chat.Title = title.String
	}
	if description.Valid {
		chat.Description = description.String
	}

	// Get members
	members, _ := s.getChatMembers(chatID)
	chat.Members = members
	chat.MemberCount = len(members)

	return &ChatResponse{
		Chat:        chat,
		UserRole:    userRole,
		CanSend:     userRole != "",
		CanAddUsers: userRole == "creator" || userRole == "admin",
	}, nil
}

func (s *ChatService) getChatMembers(chatID uuid.UUID) ([]ChatMember, error) {
	query := `
		SELECT cm.chat_id, cm.user_id, cm.role, cm.status, cm.joined_at, cm.left_at, cm.invited_by,
		       u.username, u.first_name, u.last_name
		FROM chat_members cm
		JOIN users u ON cm.user_id = u.id
		WHERE cm.chat_id = $1 AND cm.status = 'active'
		ORDER BY cm.joined_at`

	rows, err := s.db.Query(query, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ChatMember
	for rows.Next() {
		var member ChatMember
		var leftAt sql.NullTime
		var invitedBy sql.NullString

		err := rows.Scan(
			&member.ChatID, &member.UserID, &member.Role, &member.Status, &member.JoinedAt,
			&leftAt, &invitedBy, &member.Username, &member.FirstName, &member.LastName,
		)
		if err != nil {
			continue
		}

		if leftAt.Valid {
			member.LeftAt = &leftAt.Time
		}
		if invitedBy.Valid {
			if id, err := uuid.Parse(invitedBy.String); err == nil {
				member.InvitedBy = &id
			}
		}

		members = append(members, member)
	}

	return members, nil
}

func (s *ChatService) canUserSendMessage(userID uuid.UUID, chatID uuid.UUID) (bool, error) {
	query := `
		SELECT cm.role FROM chat_members cm
		JOIN chats c ON cm.chat_id = c.id
		WHERE cm.user_id = $1 AND cm.chat_id = $2 AND cm.status = 'active' AND c.is_active = true`

	var role string
	err := s.db.QueryRow(query, userID, chatID).Scan(&role)
	return err == nil && role != "", err
}

func (s *ChatService) isUserChatMember(userID uuid.UUID, chatID uuid.UUID) (bool, error) {
	query := `
		SELECT 1 FROM chat_members
		WHERE user_id = $1 AND chat_id = $2 AND status = 'active'`

	var exists int
	err := s.db.QueryRow(query, userID, chatID).Scan(&exists)
	return err == nil, nil
}

func (s *ChatService) getUserIDByUsername(username string) (uuid.UUID, error) {
	query := `SELECT id FROM users WHERE username = $1`
	var userID uuid.UUID
	err := s.db.QueryRow(query, username).Scan(&userID)
	return userID, err
}

func (s *ChatService) getUserInfo(userID uuid.UUID) (*struct {
	Username  string
	FirstName string
	LastName  string
}, error) {
	query := `SELECT username, first_name, last_name FROM users WHERE id = $1`
	var user struct {
		Username  string
		FirstName string
		LastName  string
	}
	err := s.db.QueryRow(query, userID).Scan(&user.Username, &user.FirstName, &user.LastName)
	return &user, err
}

func (s *ChatService) getLastMessage(chatID uuid.UUID) (*Message, error) {
	query := `
		SELECT id, sender_id, message_type, content, created_at
		FROM messages
		WHERE chat_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
		LIMIT 1`

	var msg Message
	err := s.db.QueryRow(query, chatID).Scan(
		&msg.ID, &msg.SenderID, &msg.MessageType, &msg.Content, &msg.CreatedAt,
	)
	return &msg, err
}

func (s *ChatService) getUnreadCount(userID uuid.UUID, chatID uuid.UUID) int {
	query := `
		SELECT COUNT(*) FROM messages m
		LEFT JOIN message_delivery md ON m.id = md.message_id AND md.user_id = $1
		WHERE m.chat_id = $2 AND m.sender_id != $1 AND m.is_deleted = false
		AND (md.status IS NULL OR md.status != 'read')`

	var count int
	s.db.QueryRow(query, userID, chatID).Scan(&count)
	return count
}

func (s *ChatService) updateChatTimestamp(chatID uuid.UUID) {
	query := `UPDATE chats SET updated_at = NOW() WHERE id = $1`
	s.db.Exec(query, chatID)
}

// Add these methods to your existing ChatService in internal/chat/service.go

// AddReaction adds a reaction to a message
func (s *ChatService) AddReaction(userID uuid.UUID, messageID uuid.UUID, chatID uuid.UUID, reactionType string) (*ReactionResponse, error) {
	// Verify user has access to this chat
	isMember, err := s.isUserChatMember(userID, chatID)
	if err != nil || !isMember {
		return nil, errors.New("access denied")
	}

	// Verify message exists in this chat
	var msgChatID uuid.UUID
	err = s.db.QueryRow("SELECT chat_id FROM messages WHERE id = $1", messageID).Scan(&msgChatID)
	if err != nil {
		return nil, errors.New("message not found")
	}
	if msgChatID != chatID {
		return nil, errors.New("message not in this chat")
	}

	// Add or update reaction (upsert)
	query := `
		INSERT INTO message_reactions (message_id, user_id, reaction_type, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (message_id, user_id, reaction_type) DO NOTHING`

	_, err = s.db.Exec(query, messageID, userID, reactionType)
	if err != nil {
		return nil, err
	}

	// Get updated reaction summary
	reactionResponse, err := s.GetMessageReactions(messageID, userID)
	if err != nil {
		return nil, err
	}

	// Broadcast reaction via WebSocket
	go s.broadcastReaction(chatID, messageID, userID, reactionType, "added")

	return reactionResponse, nil
}

// RemoveReaction removes a user's reaction from a message
func (s *ChatService) RemoveReaction(userID uuid.UUID, messageID uuid.UUID, chatID uuid.UUID, reactionType string) (*ReactionResponse, error) {
	// Verify user has access to this chat
	isMember, err := s.isUserChatMember(userID, chatID)
	if err != nil || !isMember {
		return nil, errors.New("access denied")
	}

	// Remove reaction
	query := `DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2 AND reaction_type = $3`
	result, err := s.db.Exec(query, messageID, userID, reactionType)
	if err != nil {
		return nil, err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errors.New("reaction not found")
	}

	// Get updated reaction summary
	reactionResponse, err := s.GetMessageReactions(messageID, userID)
	if err != nil {
		return nil, err
	}

	// Broadcast reaction removal via WebSocket
	go s.broadcastReaction(chatID, messageID, userID, reactionType, "removed")

	return reactionResponse, nil
}

// GetMessageReactions gets all reactions for a message
func (s *ChatService) GetMessageReactions(messageID uuid.UUID, currentUserID uuid.UUID) (*ReactionResponse, error) {
	query := `
		SELECT 
			mr.reaction_type,
			COUNT(*) as count,
			ARRAY_AGG(
				JSON_BUILD_OBJECT(
					'user_id', u.id,
					'username', u.username,
					'first_name', u.first_name
				)
			) as users,
			BOOL_OR(mr.user_id = $2) as has_reacted
		FROM message_reactions mr
		JOIN users u ON mr.user_id = u.id
		WHERE mr.message_id = $1
		GROUP BY mr.reaction_type
		ORDER BY count DESC, mr.reaction_type`

	rows, err := s.db.Query(query, messageID, currentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reactions []ReactionSummary
	totalCount := 0

	for rows.Next() {
		var reaction ReactionSummary
		var usersJSON string
		var count int

		err := rows.Scan(&reaction.ReactionType, &count, &usersJSON, &reaction.HasReacted)
		if err != nil {
			continue
		}

		reaction.Count = count
		totalCount += count

		// Parse users JSON
		var users []ReactionUser
		if err := json.Unmarshal([]byte(usersJSON), &users); err == nil {
			reaction.Users = users
		}

		reactions = append(reactions, reaction)
	}

	return &ReactionResponse{
		MessageID:  messageID,
		Reactions:  reactions,
		TotalCount: totalCount,
	}, nil
}

func (s *ChatService) broadcastReaction(chatID uuid.UUID, messageID uuid.UUID, userID uuid.UUID, reactionType string, action string) {
	if s.wsHub == nil {
		return // No WebSocket hub available
	}

	// Get user info for the broadcast
	userInfo, err := s.getUserInfo(userID)
	if err != nil {
		return
	}

	// Create WebSocket message
	wsMessage := WSMessage{
		Type:      "message_reaction",
		ChatID:    chatID,
		UserID:    userID,
		MessageID: messageID,
		Content: map[string]interface{}{
			"message_id":    messageID,
			"user_id":       userID,
			"username":      userInfo.Username,
			"first_name":    userInfo.FirstName,
			"reaction_type": reactionType,
			"action":        action, // "added" or "removed"
		},
		Timestamp: time.Now(),
	}

	// Broadcast to all users in the chat
	s.wsHub.broadcast <- wsMessage
}

// Add these methods to your existing ChatService in internal/chat/service.go

// ForwardMessages forwards one or more messages to one or more chats
func (s *ChatService) ForwardMessages(userID uuid.UUID, req *ForwardMessageRequest) (*ForwardResponse, error) {
	if len(req.MessageIDs) > 10 {
		return nil, errors.New("cannot forward more than 10 messages at once")
	}

	if len(req.ToChatIDs) > 5 {
		return nil, errors.New("cannot forward to more than 5 chats at once")
	}

	var forwardedMessages []ForwardedMessage
	var failedForwards []FailedForward

	// Process each message to each chat
	for _, messageID := range req.MessageIDs {
		for _, toChatID := range req.ToChatIDs {
			forwarded, err := s.forwardSingleMessage(userID, messageID, toChatID, req.Caption)
			if err != nil {
				failedForwards = append(failedForwards, FailedForward{
					MessageID: messageID,
					ChatID:    toChatID,
					Error:     err.Error(),
				})
				continue
			}
			forwardedMessages = append(forwardedMessages, *forwarded)
		}
	}

	return &ForwardResponse{
		ForwardedCount:    len(forwardedMessages),
		ForwardedMessages: forwardedMessages,
		FailedForwards:    failedForwards,
	}, nil
}

// forwardSingleMessage forwards a single message to a single chat
func (s *ChatService) forwardSingleMessage(userID uuid.UUID, messageID uuid.UUID, toChatID uuid.UUID, caption string) (*ForwardedMessage, error) {
	// 1. Verify user can access the original message
	originalMessage, err := s.getMessageForForwarding(userID, messageID)
	if err != nil {
		return nil, fmt.Errorf("cannot access original message: %v", err)
	}

	// 2. Verify user can send to target chat
	canSend, err := s.canUserSendMessage(userID, toChatID)
	if err != nil || !canSend {
		return nil, errors.New("cannot send to target chat")
	}

	// 3. Check forward chain depth (prevent infinite forwarding)
	depth := s.getForwardChainDepth(messageID)
	if depth > 5 {
		return nil, errors.New("message has been forwarded too many times")
	}

	// 4. Create the forwarded message
	newMessageID := uuid.New()
	now := time.Now()

	// Determine forward source (if already forwarded, use original source)
	var forwardFromUserID, forwardFromChatID, forwardFromMessageID *uuid.UUID
	var forwardDate *time.Time

	if originalMessage.ForwardFromMessageID != nil {
		// Already a forward, maintain original source
		forwardFromUserID = originalMessage.ForwardFromUserID
		forwardFromChatID = originalMessage.ForwardFromChatID
		forwardFromMessageID = originalMessage.ForwardFromMessageID
		forwardDate = originalMessage.ForwardDate
	} else {
		// First forward, set current message as source
		forwardFromUserID = &originalMessage.SenderID
		forwardFromChatID = &originalMessage.ChatID
		forwardFromMessageID = &messageID
		forwardDate = &originalMessage.CreatedAt
	}

	// 5. Insert forwarded message
	insertQuery := `
		INSERT INTO messages (
			id, chat_id, sender_id, message_type, content, caption,
			forward_from_user_id, forward_from_chat_id, forward_from_message_id, forward_date,
			file_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	messageCaption := caption
	if messageCaption == "" && originalMessage.MessageType != "text" {
		messageCaption = originalMessage.Content // Use original caption for media
	}

	_, err = s.db.Exec(insertQuery,
		newMessageID, toChatID, userID, originalMessage.MessageType, originalMessage.Content, messageCaption,
		forwardFromUserID, forwardFromChatID, forwardFromMessageID, forwardDate,
		originalMessage.FileID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create forwarded message: %v", err)
	}

	// 6. Track the forward relationship
	trackQuery := `
		INSERT INTO message_forwards (original_message_id, forwarded_message_id, forwarded_by, forwarded_to_chat_id)
		VALUES ($1, $2, $3, $4)`
	_, err = s.db.Exec(trackQuery, messageID, newMessageID, userID, toChatID)
	if err != nil {
		// Not critical if tracking fails
		log.Printf("Failed to track forward: %v", err)
	}

	// 7. Update chat timestamp
	s.updateChatTimestamp(toChatID)

	// 8. Get original sender and chat info for response
	originalSender, originalChatTitle := s.getForwardSourceInfo(forwardFromUserID, forwardFromChatID)

	// 9. Broadcast via WebSocket
	if s.wsHub != nil {
		go s.broadcastForwardedMessage(toChatID, newMessageID, userID, originalMessage)
	}

	return &ForwardedMessage{
		ID:                newMessageID,
		OriginalMessageID: messageID,
		ForwardedToChatID: toChatID,
		ForwardedBy:       userID,
		ForwardedAt:       now,
		OriginalSender:    originalSender,
		OriginalChatTitle: originalChatTitle,
		OriginalContent:   originalMessage.Content,
		OriginalCreatedAt: originalMessage.CreatedAt,
	}, nil
}

// Helper functions for forwarding

func (s *ChatService) getMessageForForwarding(userID uuid.UUID, messageID uuid.UUID) (*Message, error) {
	// Check if user has access to this message (is member of the chat)
	query := `
		SELECT m.id, m.chat_id, m.sender_id, m.message_type, m.content, m.file_id, m.created_at,
		       m.forward_from_user_id, m.forward_from_chat_id, m.forward_from_message_id, m.forward_date
		FROM messages m
		JOIN chat_members cm ON m.chat_id = cm.chat_id
		WHERE m.id = $1 AND cm.user_id = $2 AND cm.status = 'active' AND m.is_deleted = false`

	var msg Message
	var forwardFromUserID, forwardFromChatID, forwardFromMessageID sql.NullString
	var forwardDate sql.NullTime

	err := s.db.QueryRow(query, messageID, userID).Scan(
		&msg.ID, &msg.ChatID, &msg.SenderID, &msg.MessageType, &msg.Content, &msg.FileID, &msg.CreatedAt,
		&forwardFromUserID, &forwardFromChatID, &forwardFromMessageID, &forwardDate,
	)
	if err != nil {
		return nil, err
	}

	// Parse forward info if present
	if forwardFromUserID.Valid {
		if id, err := uuid.Parse(forwardFromUserID.String); err == nil {
			msg.ForwardFromUserID = &id
		}
	}
	if forwardFromChatID.Valid {
		if id, err := uuid.Parse(forwardFromChatID.String); err == nil {
			msg.ForwardFromChatID = &id
		}
	}
	if forwardFromMessageID.Valid {
		if id, err := uuid.Parse(forwardFromMessageID.String); err == nil {
			msg.ForwardFromMessageID = &id
		}
	}
	if forwardDate.Valid {
		msg.ForwardDate = &forwardDate.Time
	}

	return &msg, nil
}

func (s *ChatService) getForwardChainDepth(messageID uuid.UUID) int {
	var depth int
	query := `SELECT get_forward_chain_depth($1)`
	s.db.QueryRow(query, messageID).Scan(&depth)
	return depth
}

func (s *ChatService) getForwardSourceInfo(userID *uuid.UUID, chatID *uuid.UUID) (string, string) {
	var senderName, chatTitle string

	if userID != nil {
		userQuery := `SELECT CONCAT(first_name, ' ', last_name) FROM users WHERE id = $1`
		s.db.QueryRow(userQuery, *userID).Scan(&senderName)
	}

	if chatID != nil {
		chatQuery := `SELECT COALESCE(title, 'Private Chat') FROM chats WHERE id = $1`
		s.db.QueryRow(chatQuery, *chatID).Scan(&chatTitle)
	}

	return senderName, chatTitle
}

func (s *ChatService) broadcastForwardedMessage(chatID uuid.UUID, messageID uuid.UUID, userID uuid.UUID, originalMessage *Message) {
	if s.wsHub == nil {
		return
	}

	// Create WebSocket message for forwarded message
	wsMessage := WSMessage{
		Type:      WSMessageReceived,
		ChatID:    chatID,
		UserID:    userID,
		MessageID: messageID,
		Content: map[string]interface{}{
			"message_id":   messageID,
			"message_type": originalMessage.MessageType,
			"content":      originalMessage.Content,
			"is_forwarded": true,
			"forwarded_by": userID,
		},
		Timestamp: time.Now(),
	}

	s.wsHub.broadcast <- wsMessage
}
