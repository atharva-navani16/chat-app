package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *AuthService
}

func NewAuthHandler(authService *AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request data"})
		return
	}

	authResponse, err := h.authService.Register(&req)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, authResponse)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request data"})
		return
	}

	authResponse, err := h.authService.Login(&req)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, authResponse)
}

// Add this method to your AuthHandler in internal/auth/handler.go
func (h *AuthHandler) SearchUsers(c *gin.Context) {
	user, exists := RequireUser(c)
	if !exists {
		return
	}

	query := c.Query("q")
	if query == "" || len(query) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Search query must be at least 2 characters",
		})
		return
	}

	searchType := c.DefaultQuery("type", "all")

	results, err := h.authService.SearchUsers(query, searchType, user.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Search failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Search completed successfully",
		"data": gin.H{
			"query":       query,
			"type":        searchType,
			"results":     results,
			"total":       len(results),
			"searched_by": user.Username,
		},
	})
}
