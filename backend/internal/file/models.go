// internal/file/models.go
package file

import (
	"time"

	"github.com/google/uuid"
)

// File represents a file in the system
type File struct {
	ID           uuid.UUID `json:"id" db:"id"`
	OriginalName string    `json:"original_name" db:"original_name"`
	FileType     string    `json:"file_type" db:"file_type"` // image, video, audio, document
	MimeType     string    `json:"mime_type" db:"mime_type"`
	FileSize     int64     `json:"file_size" db:"file_size"`
	StoragePath  string    `json:"-" db:"storage_path"` // Internal storage path
	CDNUrl       string    `json:"url" db:"cdn_url"`

	// Media metadata
	Width    *int `json:"width,omitempty" db:"width"`
	Height   *int `json:"height,omitempty" db:"height"`
	Duration *int `json:"duration,omitempty" db:"duration"` // seconds for audio/video

	// Thumbnail
	ThumbnailID  *uuid.UUID `json:"thumbnail_id,omitempty" db:"thumbnail_file_id"`
	ThumbnailURL string     `json:"thumbnail_url,omitempty"`

	// Status
	ProcessingStatus string `json:"processing_status" db:"processing_status"` // pending, completed, failed

	// Metadata
	UploadedBy uuid.UUID  `json:"uploaded_by" db:"uploaded_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// FileUploadRequest for file upload
type FileUploadRequest struct {
	ChatID      uuid.UUID `form:"chat_id" binding:"required"`
	MessageType string    `form:"message_type"` // image, video, audio, document
	Caption     string    `form:"caption"`
}

// FileUploadResponse for upload response
type FileUploadResponse struct {
	File    File   `json:"file"`
	Message string `json:"message"`
}

// Supported file types and limits
const (
	MaxFileSize  = 50 * 1024 * 1024 // 50MB
	MaxImageSize = 10 * 1024 * 1024 // 10MB
	MaxVideoSize = 50 * 1024 * 1024 // 50MB
)

var AllowedImageTypes = []string{
	"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp",
}

var AllowedVideoTypes = []string{
	"video/mp4", "video/mpeg", "video/quicktime", "video/webm",
}

var AllowedAudioTypes = []string{
	"audio/mpeg", "audio/wav", "audio/ogg", "audio/m4a",
}

var AllowedDocumentTypes = []string{
	"application/pdf", "text/plain", "application/msword",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}
