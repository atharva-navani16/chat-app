package chat

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/atharva-navani16/chat-app.git/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ChatHandler struct {
	chatService *ChatService
}

func NewChatHandler(chatService *ChatService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
	}
}

// CreatePrivateChat creates a 1-on-1 chat
// POST /api/v1/chats/private
func (h *ChatHandler) CreatePrivateChat(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	var req CreatePrivateChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	chatResponse, err := h.chatService.CreatePrivateChat(user.Id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create chat",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Private chat created successfully",
		"data":    chatResponse,
	})
}

// CreateGroupChat creates a group chat
// POST /api/v1/chats/group
func (h *ChatHandler) CreateGroupChat(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	var req CreateGroupChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Validate member count
	if len(req.MemberIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "At least one member required",
		})
		return
	}

	if len(req.MemberIDs) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Maximum 200 members allowed",
		})
		return
	}

	chatResponse, err := h.chatService.CreateGroupChat(user.Id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create group chat",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Group chat created successfully",
		"data":    chatResponse,
	})
}

// SendMessage sends a message to a chat
// POST /api/v1/chats/:chat_id/messages
func (h *ChatHandler) SendMessage(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Set chat ID from URL parameter
	req.ChatID = chatID

	// Validate content
	if req.Content == "" && req.FileID == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Message content or file required",
		})
		return
	}

	message, err := h.chatService.SendMessage(user.Id, &req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "permission denied" {
			statusCode = http.StatusForbidden
		}
		c.JSON(statusCode, gin.H{
			"error":   "Failed to send message",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Message sent successfully",
		"data":    message,
	})
}

// GetMessages retrieves messages from a chat
// GET /api/v1/chats/:chat_id/messages?limit=50&offset=0
func (h *ChatHandler) GetMessages(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	// Parse query parameters
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	req := &GetMessagesRequest{
		ChatID: chatID,
		Limit:  limit,
		Offset: offset,
	}

	messagesResponse, err := h.chatService.GetMessages(user.Id, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "access denied" {
			statusCode = http.StatusForbidden
		}
		c.JSON(statusCode, gin.H{
			"error":   "Failed to get messages",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Messages retrieved successfully",
		"data":    messagesResponse,
	})
}

// GetUserChats retrieves all chats for the current user
// GET /api/v1/chats
func (h *ChatHandler) GetUserChats(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatListResponse, err := h.chatService.GetUserChats(user.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get chats",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Chats retrieved successfully",
		"data":    chatListResponse,
	})
}

// GetChatDetails gets details of a specific chat
// GET /api/v1/chats/:chat_id
func (h *ChatHandler) GetChatDetails(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	chatResponse, err := h.chatService.getChatResponse(chatID, user.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Chat not found or access denied",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Chat details retrieved successfully",
		"data":    chatResponse,
	})
}

// GetChatMembers gets members of a chat
// GET /api/v1/chats/:chat_id/members
func (h *ChatHandler) GetChatMembers(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	// Verify user is member of the chat
	isMember, err := h.chatService.isUserChatMember(user.Id, chatID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Access denied",
		})
		return
	}

	members, err := h.chatService.getChatMembers(chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get chat members",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Chat members retrieved successfully",
		"data": gin.H{
			"chat_id": chatID,
			"members": members,
			"count":   len(members),
		},
	})
}

// Placeholder handlers for future implementation

// UpdateChat updates chat details (title, description)
// PUT /api/v1/chats/:chat_id
func (h *ChatHandler) UpdateChat(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Chat update functionality coming soon",
		"chat_id":    chatID,
		"updated_by": user.Username,
	})
}

// LeaveChat allows a user to leave a chat
// POST /api/v1/chats/:chat_id/leave
func (h *ChatHandler) LeaveChat(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Leave chat functionality coming soon",
		"chat_id": chatID,
		"user":    user.Username,
	})
}

// AddMembersToGroup adds members to a group chat
// POST /api/v1/chats/:chat_id/members
func (h *ChatHandler) AddMembersToGroup(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Add members functionality coming soon",
		"chat_id":  chatID,
		"added_by": user.Username,
	})
}

// RemoveMemberFromGroup removes a member from a group chat
// DELETE /api/v1/chats/:chat_id/members/:user_id
func (h *ChatHandler) RemoveMemberFromGroup(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	memberIDStr := c.Param("user_id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Remove member functionality coming soon",
		"chat_id":    chatID,
		"member_id":  memberID,
		"removed_by": user.Username,
	})
}

// MarkMessageAsRead marks a message as read
// POST /api/v1/chats/:chat_id/messages/:message_id/read
func (h *ChatHandler) MarkMessageAsRead(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat ID",
		})
		return
	}

	messageIDStr := c.Param("message_id")
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid message ID",
		})
		return
	}

	// TODO: Implement mark as read functionality
	c.JSON(http.StatusOK, gin.H{
		"message":    "Message marked as read",
		"chat_id":    chatID,
		"message_id": messageID,
		"user":       user.Username,
	})
}

// SearchChats searches for public chats
// GET /api/v1/chats/search?q=query
func (h *ChatHandler) SearchChats(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Search query required",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Search functionality coming soon",
		"query":       query,
		"searched_by": user.Username,
		"results":     []interface{}{},
	})
}

// Add these methods to your existing ChatHandler in internal/chat/handler.go

// AddReaction adds a reaction to a message
// POST /api/v1/chats/:chat_id/messages/:message_id/reactions
func (h *ChatHandler) AddReaction(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chat ID"})
		return
	}

	messageIDStr := c.Param("message_id")
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}

	var req AddReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Validate reaction type
	validReactions := []string{"👍", "👎", "❤️", "😂", "😮", "😢", "😡", "🔥", "👏", "🎉"}
	isValid := false
	for _, valid := range validReactions {
		if req.ReactionType == valid {
			isValid = true
			break
		}
	}
	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":           "Invalid reaction type",
			"valid_reactions": validReactions,
		})
		return
	}

	reactionResponse, err := h.chatService.AddReaction(user.Id, messageID, chatID, req.ReactionType)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "access denied" {
			statusCode = http.StatusForbidden
		}
		c.JSON(statusCode, gin.H{
			"error":   "Failed to add reaction",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reaction added successfully",
		"data":    reactionResponse,
	})
}

// RemoveReaction removes a reaction from a message
// DELETE /api/v1/chats/:chat_id/messages/:message_id/reactions/:reaction_type
func (h *ChatHandler) RemoveReaction(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	chatIDStr := c.Param("chat_id")
	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chat ID"})
		return
	}

	messageIDStr := c.Param("message_id")
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}

	reactionType := c.Param("reaction_type")
	if reactionType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reaction type required"})
		return
	}

	reactionResponse, err := h.chatService.RemoveReaction(user.Id, messageID, chatID, reactionType)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "access denied" {
			statusCode = http.StatusForbidden
		} else if err.Error() == "reaction not found" {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{
			"error":   "Failed to remove reaction",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reaction removed successfully",
		"data":    reactionResponse,
	})
}

func (h *ChatHandler) GetMessageReactions(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	messageIDStr := c.Param("message_id")
	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}

	reactionResponse, err := h.chatService.GetMessageReactions(messageID, user.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get reactions",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reactions retrieved successfully",
		"data":    reactionResponse,
	})
}

// Add this method to your existing ChatHandler in internal/chat/handler.go

// ForwardMessages forwards messages to other chats
// POST /api/v1/chats/forward
func (h *ChatHandler) ForwardMessages(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	var req ForwardMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	// Validate limits
	if len(req.MessageIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "At least one message ID required",
		})
		return
	}

	if len(req.ToChatIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "At least one target chat ID required",
		})
		return
	}

	forwardResponse, err := h.chatService.ForwardMessages(user.Id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to forward messages",
			"details": err.Error(),
		})
		return
	}

	// Determine response status
	statusCode := http.StatusOK
	if forwardResponse.ForwardedCount == 0 {
		statusCode = http.StatusBadRequest
	} else if len(forwardResponse.FailedForwards) > 0 {
		statusCode = http.StatusPartialContent // 206 - some succeeded, some failed
	}

	c.JSON(statusCode, gin.H{
		"message": fmt.Sprintf("Forwarded %d of %d messages to %d chats",
			forwardResponse.ForwardedCount,
			len(req.MessageIDs)*len(req.ToChatIDs),
			len(req.ToChatIDs)),
		"data": forwardResponse,
	})
}
