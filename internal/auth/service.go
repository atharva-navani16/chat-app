package auth

import (
	"crypto/rand"
	"database/sql"
	"time"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/nacl/box"
)

type AuthService struct {
	db     *sql.DB
	rdb    *redis.Client
	config *config.Config
}

func NewAuthService(db *sql.DB, rdb *redis.Client, config *config.Config) *AuthService {
	return &AuthService{
		db:     db,
		rdb:    rdb,
		config: config,
	}
}

func (s *AuthService) Register(req *CreateUserRequest) (*AuthResponse, error) {
	// Implement registration logic here
	hashedPassword, err := s.hashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	publicKey, signedPreKey, signature := s.generateCryptoKeys()

	user, err := s.CreateUserInDB(req, hashedPassword, publicKey, signedPreKey, signature)
	if err != nil {
		return nil, err
	}

	token, expiresAt, err := s.generateJWT(user.Id)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      s.userToResponse(user),
	}, nil

}

func (s *AuthService) hashPassword(password string) (string, error) {
	byte, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(byte), err
}

func (s *AuthService) checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (s *AuthService) generateCryptoKeys() ([]byte, []byte, []byte) {
	publicKey, _, err := box.GenerateKey(rand.Reader)
	if err != nil {
		panic("Failed to generate crypto keys: " + err.Error())
	}
	signedPreKey := publicKey[:]

	signature := make([]byte, 64)
	rand.Read(signature)

	return publicKey[:], signedPreKey, signature
}

func (s *AuthService) CreateUserInDB(req *CreateUserRequest, hashedPassword string, publicKey, signedPreKey, signature []byte) (*Users, error) {
	userId := uuid.New()
	query := `
    INSERT INTO users (
        id, phone_number, username, first_name, last_name, 
        password_hash, public_key, signed_prekey, prekey_signature,
        is_public, allow_phone_discovery, last_seen_privacy,
        is_online, status, created_at, updated_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
    RETURNING *`
	var user Users
	err := s.db.QueryRow(query, userId, req.PhoneNumber, req.Username, req.FirstName, req.LastName, hashedPassword, publicKey, signedPreKey, signature, true, true, "everyone", false, "active", time.Now(), time.Now()).Scan(
		&user.Id, &user.PhoneNumber, &user.Username, &user.FirstName, &user.LastName,
		&user.Bio, &user.ProfilePhotoID, &user.IsPublic, &user.AllowPhoneDiscovery,
		&user.LastSeenPrivacy, &user.PasswordHash, &user.PublicKey, &user.SignedPrekey,
		&user.PrekeySignature, &user.IsOnline, &user.LastSeen, &user.Status,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) generateJWT(userID uuid.UUID) (string, time.Time, error) {
	// Token expires in 24 hours (or whatever you set in config)
	expiresAt := time.Now().Add(24 * time.Hour)

	// Create the claims (data inside the token)
	claims := jwt.MapClaims{
		"user_id": userID.String(),
		"exp":     expiresAt.Unix(),  // Expiration time
		"iat":     time.Now().Unix(), // Issued at time
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign it with your secret key
	tokenString, err := token.SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

func (s *AuthService) userToResponse(user *Users) UserResponse {
	return UserResponse{
		Id:             user.Id,
		PhoneNumber:    user.PhoneNumber,
		Username:       user.Username,
		FirstName:      user.FirstName,
		LastName:       user.LastName,
		Bio:            user.Bio,
		ProfilePhotoID: user.ProfilePhotoID,
	}
}
