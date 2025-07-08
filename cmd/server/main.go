// cmd/server/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/atharva-navani16/chat-app.git/internal/auth"
	"github.com/atharva-navani16/chat-app.git/internal/chat"
	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/atharva-navani16/chat-app.git/internal/shared/database"
)

func main() {
	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize database connections
	db := database.InitDB(cfg)
	rdb := database.InitRedis(cfg)
	defer db.Close()
	defer rdb.Close()

	// Set Gin mode
	gin.SetMode(cfg.GinMode)
	router := gin.Default()

	// Add CORS middleware for development
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	})

	// Initialize services
	authService := auth.NewAuthService(db, rdb, cfg)
	chatService := chat.NewChatService(db, rdb, cfg)

	// Initialize handlers
	authHandler := auth.NewAuthHandler(authService)
	chatHandler := chat.NewChatHandler(chatService)

	// Initialize JWT middleware
	jwtMiddleware := auth.NewJWTMiddleware(cfg, db)

	// Initialize WebSocket hub
	wsHub := chat.NewWSHub(rdb)
	go wsHub.Run()

	// Start Redis subscriber for cross-server communication
	ctx := context.Background()
	go wsHub.RedisSubscriber(ctx)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		connectionCount := wsHub.GetConnectionCount()
		
		if err := db.Ping(); err != nil {
			c.JSON(500, gin.H{"error": "Database not healthy"})
			return
		}
		if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(500, gin.H{"error": "Redis not healthy"})
			return
		}
		
		c.JSON(200, gin.H{
			"message": "healthy",
			"timestamp": "2025-07-08",
			"websocket_connections": connectionCount,
			"version": "1.0.0",
		})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Public auth routes (no authentication required)
		authRoutes := api.Group("/auth")
		{
			authRoutes.POST("/register", authHandler.Register)
			authRoutes.POST("/login", authHandler.Login)
		}

		// Protected user routes (authentication required)
		userRoutes := api.Group("/users")
		userRoutes.Use(jwtMiddleware.AuthRequired())
		{
			userRoutes.GET("/me", getUserProfile)
			userRoutes.PUT("/me", updateUserProfile)
			userRoutes.GET("/search", searchUsers)
		}

		// Protected chat routes (authentication required)
		chatRoutes := api.Group("/chats")
		chatRoutes.Use(jwtMiddleware.AuthRequired())
		{
			// Chat management
			chatRoutes.GET("", chatHandler.GetUserChats)                    // Get user's chats
			chatRoutes.POST("/private", chatHandler.CreatePrivateChat)      // Create private chat
			chatRoutes.POST("/group", chatHandler.CreateGroupChat)          // Create group chat
			chatRoutes.GET("/:chat_id", chatHandler.GetChatDetails)         // Get chat details
			chatRoutes.PUT("/:chat_id", chatHandler.UpdateChat)             // Update chat
			chatRoutes.POST("/:chat_id/leave", chatHandler.LeaveChat)       // Leave chat
			
			// Message management
			chatRoutes.GET("/:chat_id/messages", chatHandler.GetMessages)                     // Get messages
			chatRoutes.POST("/:chat_id/messages", chatHandler.SendMessage)                    // Send message
			chatRoutes.POST("/:chat_id/messages/:message_id/read", chatHandler.MarkMessageAsRead) // Mark as read
			
			// Member management
			chatRoutes.GET("/:chat_id/members", chatHandler.GetChatMembers)          // Get members
			chatRoutes.POST("/:chat_id/members", chatHandler.AddMembersToGroup)      // Add members
			chatRoutes.DELETE("/:chat_id/members/:user_id", chatHandler.RemoveMemberFromGroup) // Remove member
			
			// Search
			chatRoutes.GET("/search", chatHandler.SearchChats) // Search chats
		}

		// WebSocket endpoint (authentication required)
		wsRoutes := api.Group("/ws")
		wsRoutes.Use(jwtMiddleware.AuthRequired())
		{
			wsRoutes.GET("/connect", wsHub.HandleWebSocket) // WebSocket connection
		}

		// Semi-protected routes (optional authentication)
		publicRoutes := api.Group("/public")
		publicRoutes.Use(jwtMiddleware.OptionalAuth())
		{
			publicRoutes.GET("/users/:username", getPublicUserProfile)
		}
	}

	// Start server
	serverAddr := fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort)
	fmt.Printf("🚀 Chat Server starting on %s\n", serverAddr)
	fmt.Println("")
	fmt.Println("📡 ═══════════════════════════════════════════════════")
	fmt.Println("   🎯 TELEGRAM-LIKE CHAT API ENDPOINTS")
	fmt.Println("📡 ═══════════════════════════════════════════════════")
	fmt.Println("")
	fmt.Println("🔐 Authentication:")
	fmt.Println("   🔓 POST /api/v1/auth/register        - Register new user")
	fmt.Println("   🔓 POST /api/v1/auth/login           - Login user")
	fmt.Println("")
	fmt.Println("👤 User Management:")
	fmt.Println("   🔒 GET  /api/v1/users/me             - Get current user profile")
	fmt.Println("   🔒 PUT  /api/v1/users/me             - Update user profile")
	fmt.Println("   🔒 GET  /api/v1/users/search?q=name  - Search users")
	fmt.Println("")
	fmt.Println("💬 Chat Management:")
	fmt.Println("   🔒 GET  /api/v1/chats                - Get user's chats")
	fmt.Println("   🔒 POST /api/v1/chats/private        - Create private chat")
	fmt.Println("   🔒 POST /api/v1/chats/group          - Create group chat")
	fmt.Println("   🔒 GET  /api/v1/chats/:id            - Get chat details")
	fmt.Println("   🔒 PUT  /api/v1/chats/:id            - Update chat info")
	fmt.Println("   🔒 POST /api/v1/chats/:id/leave      - Leave chat")
	fmt.Println("")
	fmt.Println("📨 Messaging:")
	fmt.Println("   🔒 GET  /api/v1/chats/:id/messages   - Get chat messages")
	fmt.Println("   🔒 POST /api/v1/chats/:id/messages   - Send message")
	fmt.Println("   🔒 POST /api/v1/chats/:id/messages/:msg_id/read - Mark as read")
	fmt.Println("")
	fmt.Println("👥 Member Management:")
	fmt.Println("   🔒 GET  /api/v1/chats/:id/members    - Get chat members")
	fmt.Println("   🔒 POST /api/v1/chats/:id/members    - Add members")
	fmt.Println("   🔒 DEL  /api/v1/chats/:id/members/:user_id - Remove member")
	fmt.Println("")
	fmt.Println("🔌 Real-time:")
	fmt.Println("   🔒 WS   /api/v1/ws/connect           - WebSocket connection")
	fmt.Println("")
	fmt.Println("🔓 Public:")
	fmt.Println("   🔓 GET  /api/v1/public/users/:username - Public user profile")
	fmt.Println("   🔓 GET  /health                      - Health check")
	fmt.Println("")
	fmt.Println("📡 ═══════════════════════════════════════════════════")
	fmt.Println("🔑 Protected routes require: Authorization: Bearer <token>")
	fmt.Printf("💻 WebSocket URL: ws://localhost:%s/api/v1/ws/connect\n", cfg.ServerPort)
	fmt.Printf("🌐 API Base URL: http://localhost:%s/api/v1\n", cfg.ServerPort)
	fmt.Println("📡 ═══════════════════════════════════════════════════")
	fmt.Println("")
	
	router.Run(serverAddr)
}

// User management handlers (examples)

func getUserProfile(c *gin.Context) {
	user, exists := auth.GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found in context"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile retrieved successfully",
		"data": gin.H{
			"user": user,
			"features": gin.H{
				"messaging": true,
				"file_sharing": false, // Coming soon
				"voice_calls": false,  // Coming soon
				"video_calls": false,  // Coming soon
				"encryption": true,
			},
		},
	})
}

func updateUserProfile(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	var updateReq struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Bio       string `json:"bio"`
	}

	if err := c.ShouldBindJSON(&updateReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// TODO: Implement actual update logic in user service
	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"data": gin.H{
			"user_id": user.Id,
			"updates": updateReq,
		},
	})
}

func searchUsers(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query required"})
		return
	}

	searchType := c.DefaultQuery("type", "username") // username, name, phone

	// TODO: Implement actual search logic in user service
	c.JSON(http.StatusOK, gin.H{
		"message": "Search completed",
		"data": gin.H{
			"query":       query,
			"type":        searchType,
			"searched_by": user.Username,
			"results":     []interface{}{}, // Empty for now
			"total":       0,
		},
	})
}

func getPublicUserProfile(c *gin.Context) {
	username := c.Param("username")
	
	currentUser, isAuthenticated := auth.GetCurrentUser(c)
	
	// TODO: Implement logic to get public profile from database
	response := gin.H{
		"message": "Public profile retrieved",
		"data": gin.H{
			"username":    username,
			"is_public":   true,
			"bio":         "This is a sample bio",
			"joined_date": "2024-01-01",
		},
		"viewer_authenticated": isAuthenticated,
	}
	
	if isAuthenticated {
		response["viewer"] = currentUser.Username
	}
	
	c.JSON(http.StatusOK, response)
}