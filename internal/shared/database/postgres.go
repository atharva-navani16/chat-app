package database

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	_ "github.com/lib/pq"
)

func InitDB(cfg *config.Config) *sql.DB {
	connstring := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)
	fmt.Println(connstring)
	DB, err := sql.Open("postgres", connstring)
	if err != nil {
		log.Fatal("Error eastablishing a connection with the database ", err)
	}

	if err := DB.Ping(); err != nil {
		log.Fatal("Error pinging the database ", err)
	}

	log.Println("Database connected successfully")
	return DB
}

