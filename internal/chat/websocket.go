// internal/chat/websocket.go
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/atharva-navani16/chat-app.git/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// In production, implement proper origin checking
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSClient represents a WebSocket client connection
type WSClient struct {
	ID       string
	UserID   uuid.UUID
	Username string
	Conn     *websocket.Conn
	Send     chan WSMessage
	Hub      *WSHub

	// Connection metadata
	ConnectedAt time.Time
	LastPing    time.Time
	ChatRooms   map[uuid.UUID]bool // Track which chat rooms this client is in
	mutex       sync.RWMutex
}

// WSHub manages all WebSocket connections
type WSHub struct {
	// Registered clients by user ID
	clients map[uuid.UUID]map[string]*WSClient

	// Chat room subscriptions: chatID -> userID -> clientID
	chatRooms map[uuid.UUID]map[uuid.UUID]map[string]*WSClient

	// Channels for hub operations
	register   chan *WSClient
	unregister chan *WSClient
	broadcast  chan WSMessage

	// Redis for cross-server communication
	redis *redis.Client

	// Mutex for thread safety
	mutex sync.RWMutex
}

// NewWSHub creates a new WebSocket hub
func NewWSHub(redisClient *redis.Client) *WSHub {
	return &WSHub{
		clients:    make(map[uuid.UUID]map[string]*WSClient),
		chatRooms:  make(map[uuid.UUID]map[uuid.UUID]map[string]*WSClient),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		broadcast:  make(chan WSMessage, 256),
		redis:      redisClient,
	}
}

// Run starts the WebSocket hub
func (h *WSHub) Run() {
	log.Println("🔌 WebSocket hub started")

	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// HandleWebSocket upgrades HTTP connections to WebSocket
func (h *WSHub) HandleWebSocket(c *gin.Context) {
	// Authenticate user
	user, exists := auth.GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed: %v", err)
		return
	}

	// Create client
	client := &WSClient{
		ID:          generateClientID(),
		UserID:      user.Id,
		Username:    user.Username,
		Conn:        conn,
		Send:        make(chan WSMessage, 256),
		Hub:         h,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
		ChatRooms:   make(map[uuid.UUID]bool),
	}

	// Register client
	h.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// registerClient adds a client to the hub
func (h *WSHub) registerClient(client *WSClient) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Add to clients map
	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[string]*WSClient)
	}
	h.clients[client.UserID][client.ID] = client

	log.Printf("✅ Client connected: %s (User: %s)", client.ID, client.Username)

	// Send user online status to their contacts
	h.broadcastUserStatus(client.UserID, true)
}

// unregisterClient removes a client from the hub
func (h *WSHub) unregisterClient(client *WSClient) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Remove from clients map
	if clients, exists := h.clients[client.UserID]; exists {
		delete(clients, client.ID)
		if len(clients) == 0 {
			delete(h.clients, client.UserID)
			// User is now offline
			h.broadcastUserStatus(client.UserID, false)
		}
	}

	// Remove from chat rooms
	for chatID := range client.ChatRooms {
		if chatUsers, exists := h.chatRooms[chatID]; exists {
			if userClients, exists := chatUsers[client.UserID]; exists {
				delete(userClients, client.ID)
				if len(userClients) == 0 {
					delete(chatUsers, client.UserID)
				}
			}
		}
	}

	// Close connection
	close(client.Send)
	client.Conn.Close()

	log.Printf("❌ Client disconnected: %s (User: %s)", client.ID, client.Username)
}

// broadcastMessage sends a message to all relevant clients
func (h *WSHub) broadcastMessage(message WSMessage) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	switch message.Type {
	case WSMessageReceived:
		// Send to all clients in the chat
		if chatUsers, exists := h.chatRooms[message.ChatID]; exists {
			for userID, userClients := range chatUsers {
				if userID != message.UserID { // Don't send to sender
					for _, client := range userClients {
						select {
						case client.Send <- message:
						default:
							// Client's send channel is full, disconnect
							h.unregister <- client
						}
					}
				}
			}
		}

	case WSUserOnline, WSUserOffline:
		// Send to all contacts of the user
		h.broadcastToUserContacts(message.UserID, message)

	case WSTypingStart, WSTypingStop:
		// Send to other users in the chat
		if chatUsers, exists := h.chatRooms[message.ChatID]; exists {
			for userID, userClients := range chatUsers {
				if userID != message.UserID { // Don't send to the typing user
					for _, client := range userClients {
						select {
						case client.Send <- message:
						default:
							h.unregister <- client
						}
					}
				}
			}
		}
	}
}

// SendMessageToChat sends a message to all users in a specific chat
func (h *WSHub) SendMessageToChat(chatID uuid.UUID, message *Message) {
	wsMessage := WSMessage{
		Type:      WSMessageReceived,
		ChatID:    chatID,
		UserID:    message.SenderID,
		MessageID: message.ID,
		Content:   message,
		Timestamp: time.Now(),
	}

	h.broadcast <- wsMessage
}

// SendTypingIndicator sends typing status to chat members
func (h *WSHub) SendTypingIndicator(chatID uuid.UUID, userID uuid.UUID, username string, isTyping bool) {
	msgType := WSTypingStart
	if !isTyping {
		msgType = WSTypingStop
	}

	wsMessage := WSMessage{
		Type:   msgType,
		ChatID: chatID,
		UserID: userID,
		Content: map[string]interface{}{
			"chat_id":   chatID,
			"user_id":   userID,
			"username":  username,
			"is_typing": isTyping,
		},
		Timestamp: time.Now(),
	}

	h.broadcast <- wsMessage
}

// broadcastUserStatus sends user online/offline status to contacts
func (h *WSHub) broadcastUserStatus(userID uuid.UUID, isOnline bool) {
	msgType := WSUserOnline
	if !isOnline {
		msgType = WSUserOffline
	}

	wsMessage := WSMessage{
		Type:   msgType,
		UserID: userID,
		Content: map[string]interface{}{
			"user_id":   userID,
			"is_online": isOnline,
			"last_seen": time.Now(),
		},
		Timestamp: time.Now(),
	}

	h.broadcast <- wsMessage
}

// JoinChatRoom adds a client to a chat room
func (h *WSHub) JoinChatRoom(client *WSClient, chatID uuid.UUID) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Initialize chat room if it doesn't exist
	if h.chatRooms[chatID] == nil {
		h.chatRooms[chatID] = make(map[uuid.UUID]map[string]*WSClient)
	}

	// Initialize user's clients in this chat room
	if h.chatRooms[chatID][client.UserID] == nil {
		h.chatRooms[chatID][client.UserID] = make(map[string]*WSClient)
	}

	// Add client to chat room
	h.chatRooms[chatID][client.UserID][client.ID] = client

	// Track chat room in client
	client.mutex.Lock()
	client.ChatRooms[chatID] = true
	client.mutex.Unlock()

	log.Printf("👥 Client %s joined chat %s", client.ID, chatID)
}

// LeaveChatRoom removes a client from a chat room
func (h *WSHub) LeaveChatRoom(client *WSClient, chatID uuid.UUID) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if chatUsers, exists := h.chatRooms[chatID]; exists {
		if userClients, exists := chatUsers[client.UserID]; exists {
			delete(userClients, client.ID)
			if len(userClients) == 0 {
				delete(chatUsers, client.UserID)
			}
		}
	}

	// Remove from client's tracking
	client.mutex.Lock()
	delete(client.ChatRooms, chatID)
	client.mutex.Unlock()

	log.Printf("👋 Client %s left chat %s", client.ID, chatID)
}

// broadcastToUserContacts sends a message to all contacts of a user
func (h *WSHub) broadcastToUserContacts(userID uuid.UUID, message WSMessage) {
	// TODO: Implement contact-based broadcasting
	// This would require getting the user's contacts from the database
}

// Client methods

// readPump handles incoming WebSocket messages
func (c *WSClient) readPump() {
	defer func() {
		c.Hub.unregister <- c
	}()

	// Set read deadline and pong handler
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.LastPing = time.Now()
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var message WSMessage
		err := c.Conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming message
		c.handleIncomingMessage(message)
	}
}

// writePump handles outgoing WebSocket messages
func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteJSON(message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleIncomingMessage processes messages received from the client
func (c *WSClient) handleIncomingMessage(message WSMessage) {
	switch message.Type {
	case WSTypingStart:
		// Handle typing indicator
		if message.ChatID != uuid.Nil {
			c.Hub.SendTypingIndicator(message.ChatID, c.UserID, c.Username, true)
		}

	case WSTypingStop:
		// Handle stop typing
		if message.ChatID != uuid.Nil {
			c.Hub.SendTypingIndicator(message.ChatID, c.UserID, c.Username, false)
		}

	case WSMessageRead:
		// Handle message read receipt
		// TODO: Update message read status in database

	default:
		log.Printf("Unknown WebSocket message type: %s", message.Type)
	}
}

// GetOnlineUsers returns online users in a chat
func (h *WSHub) GetOnlineUsers(chatID uuid.UUID) []uuid.UUID {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var onlineUsers []uuid.UUID
	if chatUsers, exists := h.chatRooms[chatID]; exists {
		for userID := range chatUsers {
			onlineUsers = append(onlineUsers, userID)
		}
	}

	return onlineUsers
}

// IsUserOnline checks if a user is currently online
func (h *WSHub) IsUserOnline(userID uuid.UUID) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients, exists := h.clients[userID]
	return exists && len(clients) > 0
}

// GetConnectionCount returns the total number of active connections
func (h *WSHub) GetConnectionCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	count := 0
	for _, userClients := range h.clients {
		count += len(userClients)
	}
	return count
}

// Utility functions

func generateClientID() string {
	return fmt.Sprintf("client_%s_%d", uuid.New().String()[:8], time.Now().Unix())
}

// RedisSubscriber handles Redis pub/sub for cross-server communication
func (h *WSHub) RedisSubscriber(ctx context.Context) {
	pubsub := h.redis.Subscribe(ctx, "chat_messages", "user_status", "typing_indicators")
	defer pubsub.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-pubsub.Channel():
			var wsMessage WSMessage
			if err := json.Unmarshal([]byte(msg.Payload), &wsMessage); err == nil {
				h.broadcast <- wsMessage
			}
		}
	}
}
