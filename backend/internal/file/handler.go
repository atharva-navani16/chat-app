// internal/file/handler.go
package file

import (
	"net/http"
	"strconv"

	"github.com/atharva-navani16/chat-app.git/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FileHandler struct {
	fileService *FileService
}

func NewFileHandler(fileService *FileService) *FileHandler {
	return &FileHandler{
		fileService: fileService,
	}
}

// UploadFile handles file uploads
// POST /api/v1/files/upload
func (h *FileHandler) UploadFile(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	// Parse multipart form
	err := c.Request.ParseMultipartForm(MaxFileSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Failed to parse form data",
			"details": err.Error(),
		})
		return
	}

	// Get file from form
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "No file provided",
			"details": err.Error(),
		})
		return
	}

	// Get chat ID from form
	chatIDStr := c.PostForm("chat_id")
	if chatIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "chat_id is required",
		})
		return
	}

	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chat_id format",
		})
		return
	}

	// Upload file
	file, err := h.fileService.UploadFile(fileHeader, user.Id, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to upload file",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "File uploaded successfully",
		"data": FileUploadResponse{
			File:    *file,
			Message: "File ready for messaging",
		},
	})
}

// GetFile retrieves file metadata
// GET /api/v1/files/:file_id
func (h *FileHandler) GetFile(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	fileIDStr := c.Param("file_id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid file ID",
		})
		return
	}

	file, err := h.fileService.GetFile(fileID, user.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "File not found",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "File retrieved successfully",
		"data":    file,
	})
}

// DownloadFile serves file content for download
// GET /api/v1/files/:file_id/download
func (h *FileHandler) DownloadFile(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	fileIDStr := c.Param("file_id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid file ID",
		})
		return
	}

	// Get file content
	reader, file, err := h.fileService.GetFileContent(fileID, user.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "File not found",
			"details": err.Error(),
		})
		return
	}

	// Set headers for download
	c.Header("Content-Type", file.MimeType)
	c.Header("Content-Disposition", `attachment; filename="`+file.OriginalName+`"`)
	c.Header("Content-Length", strconv.FormatInt(file.FileSize, 10))

	// Stream file content
	c.DataFromReader(http.StatusOK, file.FileSize, file.MimeType, reader, nil)
}

// DeleteFile deletes a file
// DELETE /api/v1/files/:file_id
func (h *FileHandler) DeleteFile(c *gin.Context) {
	user, exists := auth.RequireUser(c)
	if !exists {
		return
	}

	fileIDStr := c.Param("file_id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid file ID",
		})
		return
	}

	err = h.fileService.DeleteFile(fileID, user.Id)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "access denied" {
			statusCode = http.StatusForbidden
		}
		c.JSON(statusCode, gin.H{
			"error":   "Failed to delete file",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "File deleted successfully",
	})
}
