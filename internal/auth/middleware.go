// internal/auth/middleware.go
package auth

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWTMiddleware struct {
	config *config.Config
	db     *sql.DB
}

// NewJWTMiddleware creates a new JWT middleware
func NewJWTMiddleware(config *config.Config, db *sql.DB) *JWTMiddleware {
	return &JWTMiddleware{
		config: config,
		db:     db,
	}
}

// AuthRequired is the main JWT authentication middleware
func (m *JWTMiddleware) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from header
		token, err := m.extractTokenFromHeader(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
				"code":  "MISSING_TOKEN",
			})
			c.Abort()
			return
		}

		// Validate and parse token
		claims, err := m.validateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired token",
				"code":  "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// Get user ID from claims
		userIDStr, ok := claims["user_id"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token claims",
				"code":  "INVALID_CLAIMS",
			})
			c.Abort()
			return
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid user ID in token",
				"code":  "INVALID_USER_ID",
			})
			c.Abort()
			return
		}

		// Verify user still exists and is active
		user, err := m.getUserByID(userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not found or inactive",
				"code":  "USER_NOT_FOUND",
			})
			c.Abort()
			return
		}

		// Store user in context for handlers to use
		c.Set("user", user)
		c.Set("user_id", userID)

		// Continue to next handler
		c.Next()
	}
}

// OptionalAuth middleware that doesn't block if no token provided
func (m *JWTMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to extract token
		token, err := m.extractTokenFromHeader(c)
		if err != nil {
			// No token provided, continue without user context
			c.Next()
			return
		}

		// Validate token if provided
		claims, err := m.validateToken(token)
		if err != nil {
			// Invalid token, continue without user context
			c.Next()
			return
		}

		// Extract user ID
		userIDStr, ok := claims["user_id"].(string)
		if ok {
			if userID, err := uuid.Parse(userIDStr); err == nil {
				if user, err := m.getUserByID(userID); err == nil {
					// Store user in context if everything is valid
					c.Set("user", user)
					c.Set("user_id", userID)
				}
			}
		}

		c.Next()
	}
}

// extractTokenFromHeader extracts JWT token from Authorization header
func (m *JWTMiddleware) extractTokenFromHeader(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", jwt.ErrTokenMalformed
	}

	// Check for "Bearer " prefix
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", jwt.ErrTokenMalformed
	}

	// Extract token (remove "Bearer " prefix)
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return "", jwt.ErrTokenMalformed
	}

	return token, nil
}

// validateToken validates JWT token and returns claims
func (m *JWTMiddleware) validateToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(m.config.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	// Check if token is valid
	if !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

// getUserByID fetches user from database by ID
// Replace the getUserByID function in middleware.go with this:
func (m *JWTMiddleware) getUserByID(userID uuid.UUID) (*UserResponse, error) {
	// Simple query - only get the essential fields that we know exist
	query := `
		SELECT id, phone_number, username, first_name, last_name
		FROM users 
		WHERE id = $1`

	var user UserResponse
	err := m.db.QueryRow(query, userID).Scan(
		&user.Id,
		&user.PhoneNumber,
		&user.Username,
		&user.FirstName,
		&user.LastName,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("database error: %v", err)
	}

	// Set default values for optional fields
	user.Bio = ""
	user.ProfilePhotoID = uuid.Nil

	return &user, nil
}

// Helper functions for handlers to get user from context

// GetCurrentUser extracts the current user from gin context
func GetCurrentUser(c *gin.Context) (*UserResponse, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}

	userResponse, ok := user.(*UserResponse)
	return userResponse, ok
}

// GetCurrentUserID extracts the current user ID from gin context
func GetCurrentUserID(c *gin.Context) (uuid.UUID, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}

	id, ok := userID.(uuid.UUID)
	return id, ok
}

// RequireUser ensures user is authenticated (for use in handlers)
func RequireUser(c *gin.Context) (*UserResponse, bool) {
	user, exists := GetCurrentUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
			"code":  "AUTH_REQUIRED",
		})
		return nil, false
	}
	return user, true
}
