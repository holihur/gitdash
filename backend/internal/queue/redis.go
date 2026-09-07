package queue

import (
	"github.com/redis/go-redis/v9"
)

// NewRedisClient 按统一约定（addr/password/db）创建 redis 客户端（runner hub 等共用）。
func NewRedisClient(addr, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
}
