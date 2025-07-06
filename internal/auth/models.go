package auth

import (
	"time"

	"github.com/google/uuid"
)

type Users struct {
	Id             uuid.UUID `json:"id"`
	PhoneNumber    string    `json:"phone_number"`
	Username       string    `json:"username"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Bio            string    `json:"bio"`
	ProfilePhotoID uuid.UUID `json:"profile_photo_id"`

	// Privacy settings
	IsPublic            bool   `json:"is_public"` // Can be found by username
	AllowPhoneDiscovery bool   `json:"allow_phone_discovery"`
	LastSeenPrivacy     string `json:"last_seen_privacy"`

	// Security
	PasswordHash    string `json:"password_hash"`
	PublicKey       []byte `json:"public_key"`
	SignedPrekey    []byte `json:"signed_prekey"`
	PrekeySignature []byte `json:"prekey_signature"`

	// Status
	IsOnline bool      `json:"is_online"`
	LastSeen time.Time `json:"last_seen"`
	Status   string    `json:"status"` // active, deleted, banned

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserResponse struct {
	Id             uuid.UUID `json:"id"`
	PhoneNumber    string    `json:"phone_number"`
	Username       string    `json:"username"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Bio            string    `json:"bio"`
	ProfilePhotoID uuid.UUID `json:"profile_photo_id"`
}

type CreateUserRequest struct {
	PhoneNumber string `json:"phone_number" binding:"required"`
	Username    string `json:"username" binding:"required"`
	FirstName   string `json:"first_name" binding:"required"`
	LastName    string `json:"last_name" binding:"required"`
	Password    string `json:"password" binding:"required"`
}

type LoginRequest struct {
    PhoneNumber string `json:"phone_number,omitempty"`
    Username    string `json:"username,omitempty"`
    Password    string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token        string      `json:"token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresAt    time.Time   `json:"expires_at"`
	User         UserResponse `json:"user"`
}
