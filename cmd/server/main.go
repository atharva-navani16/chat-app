package main

import (
	"fmt"
	"log"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/atharva-navani16/chat-app.git/internal/shared/database"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	db := database.InitDB(cfg)
	rdb := database.InitRedis(cfg)
	defer db.Close()
	defer rdb.Close()
	gin.SetMode(cfg.GinMode)
	router := gin.Default()

	healthhandler := func(c *gin.Context) {
		err := db.Ping()
		if err != nil {
			c.JSON(500, gin.H{
				"error": "Database not healthy",
			})
			return
		}

		err = rdb.Ping(c.Request.Context()).Err()
		if err != nil {
			c.JSON(500, gin.H{
				"error": "Redis Not healthy",
			})
			return
		}

		c.JSON(200, gin.H{
			"message": "healthy",
		})
	}

	serveAdd := fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort)
	router.GET("/health", healthhandler)
	fmt.Println("starting server on  ", serveAdd)
	router.Run(serveAdd)

}
