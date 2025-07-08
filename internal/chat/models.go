package chat

import (
	"time"

	"github.com/google/uuid"
)

type Chat struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Type        string    `json:"type" db:"type"`
	Title       string    `json:"title,omitempty" db:"title"`
	Description string    `json:"description,omitempty" db:"description"`
	CreatorID   uuid.UUID `json:"creator_id" db:"creator_id"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`

	// Additional fields for response
	LastMessage *Message     `json:"last_message,omitempty"`
	UnreadCount int          `json:"unread_count,omitempty"`
	MemberCount int          `json:"member_count,omitempty"`
	Members     []ChatMember `json:"members,omitempty"`
}

// ChatMember represents a user's membership in a chat
type ChatMember struct {
	ChatID    uuid.UUID  `json:"chat_id" db:"chat_id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	Role      string     `json:"role" db:"role"`     // creator, admin, member
	Status    string     `json:"status" db:"status"` // active, left, kicked, banned
	JoinedAt  time.Time  `json:"joined_at" db:"joined_at"`
	LeftAt    *time.Time `json:"left_at,omitempty" db:"left_at"`
	InvitedBy *uuid.UUID `json:"invited_by,omitempty" db:"invited_by"`

	// User info for response
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// Message represents a chat message
type Message struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	ChatID           uuid.UUID  `json:"chat_id" db:"chat_id"`
	SenderID         uuid.UUID  `json:"sender_id" db:"sender_id"`
	MessageType      string     `json:"message_type" db:"message_type"` // text, image, file, etc.
	Content          string     `json:"content,omitempty" db:"content"` // Decrypted content for response
	EncryptedContent []byte     `json:"-" db:"encrypted_content"`       // Encrypted storage
	Nonce            []byte     `json:"-" db:"nonce"`                   // Encryption nonce
	FileID           *uuid.UUID `json:"file_id,omitempty" db:"file_id"`
	ReplyToMessageID *uuid.UUID `json:"reply_to_message_id,omitempty" db:"reply_to_message_id"`
	IsEdited         bool       `json:"is_edited" db:"is_edited"`
	IsDeleted        bool       `json:"is_deleted" db:"is_deleted"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	EditedAt         *time.Time `json:"edited_at,omitempty" db:"edited_at"`

	// Additional fields for response
	SenderUsername string   `json:"sender_username,omitempty"`
	SenderName     string   `json:"sender_name,omitempty"`
	ReplyToMessage *Message `json:"reply_to_message,omitempty"`
}

// Request/Response structs

// CreatePrivateChatRequest for creating 1-on-1 chats
type CreatePrivateChatRequest struct {
	UserID   uuid.UUID `json:"user_id" binding:"required"`
	Username string    `json:"username,omitempty"` // Alternative to user_id
}

// CreateGroupChatRequest for creating group chats
type CreateGroupChatRequest struct {
	Title       string      `json:"title" binding:"required"`
	Description string      `json:"description,omitempty"`
	MemberIDs   []uuid.UUID `json:"member_ids" binding:"required,min=1"`
}

// SendMessageRequest for sending messages
type SendMessageRequest struct {
	ChatID           uuid.UUID  `json:"chat_id" binding:"required"`
	Content          string     `json:"content" binding:"required"`
	MessageType      string     `json:"message_type,omitempty"` // defaults to "text"
	ReplyToMessageID *uuid.UUID `json:"reply_to_message_id,omitempty"`
	FileID           *uuid.UUID `json:"file_id,omitempty"`
}

// GetMessagesRequest for pagination
type GetMessagesRequest struct {
	ChatID   uuid.UUID  `json:"chat_id" binding:"required"`
	Limit    int        `json:"limit,omitempty"`     // default 50
	Offset   int        `json:"offset,omitempty"`    // default 0
	BeforeID *uuid.UUID `json:"before_id,omitempty"` // for cursor-based pagination
}

// Response structs

// ChatResponse represents chat data in API responses
type ChatResponse struct {
	Chat        Chat   `json:"chat"`
	UserRole    string `json:"user_role"`
	CanSend     bool   `json:"can_send"`
	CanAddUsers bool   `json:"can_add_users"`
}

// MessagesResponse for paginated message lists
type MessagesResponse struct {
	Messages   []Message `json:"messages"`
	HasMore    bool      `json:"has_more"`
	TotalCount int       `json:"total_count"`
	NextOffset int       `json:"next_offset,omitempty"`
}

// ChatListResponse for user's chat list
type ChatListResponse struct {
	Chats      []Chat `json:"chats"`
	TotalCount int    `json:"total_count"`
}

// WebSocket message types
type WSMessageType string

const (
	WSMessageReceived WSMessageType = "message_received"
	WSMessageSent     WSMessageType = "message_sent"
	WSTypingStart     WSMessageType = "typing_start"
	WSTypingStop      WSMessageType = "typing_stop"
	WSUserOnline      WSMessageType = "user_online"
	WSUserOffline     WSMessageType = "user_offline"
	WSMessageRead     WSMessageType = "message_read"
)

// WSMessage represents WebSocket messages
type WSMessage struct {
	Type      WSMessageType `json:"type"`
	ChatID    uuid.UUID     `json:"chat_id,omitempty"`
	UserID    uuid.UUID     `json:"user_id,omitempty"`
	MessageID uuid.UUID     `json:"message_id,omitempty"`
	Content   interface{}   `json:"content,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}
