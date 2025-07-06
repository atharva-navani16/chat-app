package database

import (
	"context"
	"fmt"
	"strconv"

	"github.com/atharva-navani16/chat-app.git/internal/config"
	"github.com/redis/go-redis/v9"
)

func InitRedis(cfg *config.Config) *redis.Client {
	dbNum, err := strconv.Atoi(cfg.RedisDB)
	if err != nil {
		panic("Invalid RedisDB value: " + err.Error())
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost,
		Password: cfg.RedisPassword,
		DB:       dbNum,
		Protocol: 2,
	})
	ctx := context.Background()

	err = rdb.Set(ctx, "foo", "bar", 0).Err()
	if err != nil {
		panic(err)
	}
	val, err := rdb.Get(ctx, "foo").Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("foo", val)

	return rdb
}
