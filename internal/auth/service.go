package auth

import (
	"crypto/rand"
	"database/sql"
	"errors"
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

// NewAuthService creates a new auth service
func NewAuthService(db *sql.DB, rdb *redis.Client, config *config.Config) *AuthService {
	return &AuthService{
		db:     db,
		rdb:    rdb,
		config: config,
	}
}

// Register creates a new user account
func (s *AuthService) Register(req *CreateUserRequest) (*AuthResponse, error) {
	// Step 1: Hash password
	hashedPassword, err := s.hashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// Step 2: Generate crypto keys
	publicKey, signedPreKey, signature := s.generateCryptoKeys()

	// Step 3: Save to database
	user, err := s.createUserInDB(req, hashedPassword, publicKey, signedPreKey, signature)
	if err != nil {
		return nil, err
	}

	// Step 4: Generate JWT token
	token, expiresAt, err := s.generateJWT(user.Id)
	if err != nil {
		return nil, err
	}

	// Step 5: Return response
	return &AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      s.userToResponse(user),
	}, nil
}

// Login authenticates a user and returns a token
func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	// Step 1: Find user by phone or username
	user, err := s.findUserByCredentials(req)
	if err != nil {
		return nil, err
	}

	// Step 2: Check password
	if !s.checkPassword(req.Password, user.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}

	// Step 3: Generate JWT
	token, expiresAt, err := s.generateJWT(user.Id)
	if err != nil {
		return nil, err
	}

	// Step 4: Return response
	return &AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      s.userToResponse(user),
	}, nil
}

// hashPassword hashes a plain text password
func (s *AuthService) hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// checkPassword verifies a password against its hash
func (s *AuthService) checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// generateCryptoKeys generates encryption keys for the user
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

// createUserInDB saves a new user to the database
func (s *AuthService) createUserInDB(req *CreateUserRequest, hashedPassword string, publicKey, signedPreKey, signature []byte) (*Users, error) {
	userId := uuid.New()
	now := time.Now()

	query := `
        INSERT INTO users (
            id, phone_number, username, first_name, last_name, 
            password_hash, public_key, signed_prekey, prekey_signature,
            created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := s.db.Exec(
		query,
		userId, req.PhoneNumber, req.Username, req.FirstName, req.LastName,
		hashedPassword, publicKey, signedPreKey, signature, now, now,
	)

	if err != nil {
		return nil, err
	}

	// Return a simple user object
	return &Users{
		Id:          userId,
		PhoneNumber: req.PhoneNumber,
		Username:    req.Username,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Bio:         "", // Default empty
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// generateJWT creates a JWT token for the user
func (s *AuthService) generateJWT(userID uuid.UUID) (string, time.Time, error) {
	// Token expires in 24 hours
	expiresAt := time.Now().Add(24 * time.Hour)

	// Create the claims (data inside the token)
	claims := jwt.MapClaims{
		"user_id": userID.String(),
		"exp":     expiresAt.Unix(), // Expiration time
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

// userToResponse converts internal User struct to safe UserResponse
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

// findUserByCredentials finds a user by phone number or username
func (s *AuthService) findUserByCredentials(req *LoginRequest) (*Users, error) {
	var query string
	var param string

	// Decide whether to search by phone or username
	if req.PhoneNumber != "" {
		query = "SELECT id, phone_number, username, first_name, last_name, password_hash, created_at FROM users WHERE phone_number = $1"
		param = req.PhoneNumber
	} else if req.Username != "" {
		query = "SELECT id, phone_number, username, first_name, last_name, password_hash, created_at FROM users WHERE username = $1"
		param = req.Username
	} else {
		return nil, errors.New("phone number or username required")
	}

	var user Users
	err := s.db.QueryRow(query, param).Scan(
		&user.Id, &user.PhoneNumber, &user.Username,
		&user.FirstName, &user.LastName, &user.PasswordHash, &user.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return &user, nil
}