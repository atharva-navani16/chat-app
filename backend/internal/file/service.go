// internal/file/service.go
package file

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"time"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type FileService struct {
	db          *sql.DB
	minioClient *minio.Client
	config      *config.Config
	bucketName  string
}

func NewFileService(db *sql.DB, config *config.Config) (*FileService, error) {
	// Initialize MinIO client
	minioClient, err := minio.New(config.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinioAccessKey, config.MinioSecretKey, ""),
		Secure: false, // Set to true for HTTPS
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}

	bucketName := "chat-files"

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %v", err)
	}

	if !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %v", err)
		}
	}

	return &FileService{
		db:          db,
		minioClient: minioClient,
		config:      config,
		bucketName:  bucketName,
	}, nil
}

// UploadFile uploads a file and returns file metadata
func (s *FileService) UploadFile(fileHeader *multipart.FileHeader, userID uuid.UUID, chatID uuid.UUID) (*File, error) {
	// Validate file
	if err := s.validateFile(fileHeader); err != nil {
		return nil, err
	}

	// Open uploaded file
	uploadedFile, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %v", err)
	}
	defer uploadedFile.Close()

	// Generate file ID and storage path
	fileID := uuid.New()
	fileExtension := filepath.Ext(fileHeader.Filename)
	storagePath := fmt.Sprintf("files/%s/%s/%s%s",
		userID.String(),
		time.Now().Format("2006/01/02"),
		fileID.String(),
		fileExtension)

	// Determine file type
	fileType := s.determineFileType(fileHeader.Header.Get("Content-Type"))

	// Read file content
	fileContent, err := io.ReadAll(uploadedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Upload to MinIO
	_, err = s.minioClient.PutObject(
		context.Background(),
		s.bucketName,
		storagePath,
		bytes.NewReader(fileContent),
		fileHeader.Size,
		minio.PutObjectOptions{
			ContentType: fileHeader.Header.Get("Content-Type"),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upload to MinIO: %v", err)
	}

	// Generate CDN URL
	cdnURL := fmt.Sprintf("http://%s/%s/%s", s.config.MinioEndpoint, s.bucketName, storagePath)

	// Create file record
	file := &File{
		ID:               fileID,
		OriginalName:     fileHeader.Filename,
		FileType:         fileType,
		MimeType:         fileHeader.Header.Get("Content-Type"),
		FileSize:         fileHeader.Size,
		StoragePath:      storagePath,
		CDNUrl:           cdnURL,
		ProcessingStatus: "completed",
		UploadedBy:       userID,
		CreatedAt:        time.Now(),
	}

	// Extract metadata for images/videos
	if fileType == "image" {
		width, height := s.extractImageDimensions(fileContent)
		file.Width = &width
		file.Height = &height
	}

	// Store in database
	err = s.storeFileMetadata(file)
	if err != nil {
		// Try to delete from MinIO if database fails
		s.minioClient.RemoveObject(context.Background(), s.bucketName, storagePath, minio.RemoveObjectOptions{})
		return nil, fmt.Errorf("failed to store file metadata: %v", err)
	}

	return file, nil
}

// GetFile retrieves file metadata
func (s *FileService) GetFile(fileID uuid.UUID, userID uuid.UUID) (*File, error) {
	query := `
		SELECT f.id, f.original_name, f.file_type, f.mime_type, f.file_size,
		       f.storage_path, f.cdn_url, f.width, f.height, f.duration,
		       f.thumbnail_file_id, f.processing_status, f.uploaded_by, f.created_at, f.expires_at
		FROM files f
		WHERE f.id = $1`

	var file File
	var width, height, duration sql.NullInt32
	var thumbnailID sql.NullString
	var expiresAt sql.NullTime

	err := s.db.QueryRow(query, fileID).Scan(
		&file.ID, &file.OriginalName, &file.FileType, &file.MimeType, &file.FileSize,
		&file.StoragePath, &file.CDNUrl, &width, &height, &duration,
		&thumbnailID, &file.ProcessingStatus, &file.UploadedBy, &file.CreatedAt, &expiresAt,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if width.Valid {
		w := int(width.Int32)
		file.Width = &w
	}
	if height.Valid {
		h := int(height.Int32)
		file.Height = &h
	}
	if duration.Valid {
		d := int(duration.Int32)
		file.Duration = &d
	}
	if thumbnailID.Valid {
		if id, err := uuid.Parse(thumbnailID.String); err == nil {
			file.ThumbnailID = &id
		}
	}
	if expiresAt.Valid {
		file.ExpiresAt = &expiresAt.Time
	}

	return &file, nil
}

// GetFileContent retrieves file content for download
func (s *FileService) GetFileContent(fileID uuid.UUID, userID uuid.UUID) (io.Reader, *File, error) {
	// Get file metadata
	file, err := s.GetFile(fileID, userID)
	if err != nil {
		return nil, nil, err
	}

	// Get file content from MinIO
	object, err := s.minioClient.GetObject(
		context.Background(),
		s.bucketName,
		file.StoragePath,
		minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get file from storage: %v", err)
	}

	return object, file, nil
}

// DeleteFile deletes a file
func (s *FileService) DeleteFile(fileID uuid.UUID, userID uuid.UUID) error {
	// Get file metadata
	file, err := s.GetFile(fileID, userID)
	if err != nil {
		return err
	}

	// Check if user owns the file
	if file.UploadedBy != userID {
		return fmt.Errorf("access denied")
	}

	// Delete from MinIO
	err = s.minioClient.RemoveObject(
		context.Background(),
		s.bucketName,
		file.StoragePath,
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to delete from storage: %v", err)
	}

	// Delete from database
	query := `DELETE FROM files WHERE id = $1 AND uploaded_by = $2`
	_, err = s.db.Exec(query, fileID, userID)
	return err
}

// Helper functions

func (s *FileService) validateFile(fileHeader *multipart.FileHeader) error {
	// Check file size
	if fileHeader.Size > MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", fileHeader.Size, MaxFileSize)
	}

	// Check content type
	contentType := fileHeader.Header.Get("Content-Type")
	if !s.isAllowedContentType(contentType) {
		return fmt.Errorf("unsupported file type: %s", contentType)
	}

	return nil
}

func (s *FileService) isAllowedContentType(contentType string) bool {
	allowedTypes := append(AllowedImageTypes, AllowedVideoTypes...)
	allowedTypes = append(allowedTypes, AllowedAudioTypes...)
	allowedTypes = append(allowedTypes, AllowedDocumentTypes...)

	for _, allowed := range allowedTypes {
		if contentType == allowed {
			return true
		}
	}
	return false
}

func (s *FileService) determineFileType(contentType string) string {
	for _, imageType := range AllowedImageTypes {
		if contentType == imageType {
			return "image"
		}
	}
	for _, videoType := range AllowedVideoTypes {
		if contentType == videoType {
			return "video"
		}
	}
	for _, audioType := range AllowedAudioTypes {
		if contentType == audioType {
			return "audio"
		}
	}
	return "document"
}

func (s *FileService) extractImageDimensions(content []byte) (int, int) {
	// Basic image dimension extraction - would use proper image libraries in production
	// For now, return default values
	return 0, 0
}

func (s *FileService) storeFileMetadata(file *File) error {
	query := `
		INSERT INTO files (
			id, original_name, file_type, mime_type, file_size,
			storage_path, cdn_url, width, height, duration,
			processing_status, uploaded_by, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err := s.db.Exec(query,
		file.ID, file.OriginalName, file.FileType, file.MimeType, file.FileSize,
		file.StoragePath, file.CDNUrl, file.Width, file.Height, file.Duration,
		file.ProcessingStatus, file.UploadedBy, file.CreatedAt, file.ExpiresAt,
	)
	return err
}
