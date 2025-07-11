package config

import (
	"fmt"

	"github.com/joho/godotenv"
	"github.com/caarlos0/env/v10"
)

type Config struct {
	// Database Configuration
	DBHost     string `env:"DB_HOST"`
	DBPort     string `env:"DB_PORT"`
	DBName     string `env:"DB_NAME"`
	DBUser     string `env:"DB_USER"`
	DBPassword string `env:"DB_PASSWORD"`
	DBSSLMode  string `env:"DB_SSL_MODE"`

	// Redis Configuration
	RedisHost     string `env:"REDIS_HOST"`
	RedisPort     string `env:"REDIS_PORT"`
	RedisPassword string `env:"REDIS_PASSWORD"`
	RedisDB       string `env:"REDIS_DB"`

	// Server Configuration
	ServerPort string `env:"SERVER_PORT"`
	ServerHost string `env:"SERVER_HOST"`
	GinMode    string `env:"GIN_MODE"`

	// JWT Configuration
	JWTSecret        string `env:"JWT_SECRET"`
	JWTExpiry        string `env:"JWT_EXPIRY"`
	JWTRefreshExpiry string `env:"JWT_REFRESH_EXPIRY"`

	// File Storage (MinIO/S3)
	MinioEndpoint  string `env:"MINIO_ENDPOINT"`
	MinioAccessKey string `env:"MINIO_ACCESS_KEY"`
	MinioSecretKey string `env:"MINIO_SECRET_KEY"`
	MinioBucket    string `env:"MINIO_BUCKET"`
	MinioUseSSL    string `env:"MINIO_USE_SSL"`

	// SMS Service (for phone verification - add your provider)
	SMSProvider       string `env:"SMS_PROVIDER"`
	TwilioAccountSID  string `env:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken   string `env:"TWILIO_AUTH_TOKEN"`
	TwilioPhoneNumber string `env:"TWILIO_PHONE_NUMBER"`

	// Encryption
	EncryptionKey string `env:"ENCRYPTION_KEY"`

	// Development Settings
	LogLevel                   string `env:"LOG_LEVEL"`
	EnableCORS                 string `env:"ENABLE_CORS"`
	RateLimitEnabled           string `env:"RATE_LIMIT_ENABLED"`
	RateLimitRequestsPerMinute string `env:"RATE_LIMIT_REQUESTS_PER_MINUTE"`
}

func LoadConfig() (*Config, error) {
	config := &Config{}
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	if err := env.Parse(config); err != nil {
		return nil, fmt.Errorf("error parsing environment variables: %w", err)
	}
	return config, nil


}
