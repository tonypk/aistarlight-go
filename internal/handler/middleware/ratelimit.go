package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
)

// RateLimit implements a Redis-backed sliding window rate limiter.
func RateLimit(rdb *redis.Client, rps, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use user ID if authenticated, otherwise IP
		var key string
		if uid, exists := c.Get(string(UserIDKey)); exists {
			key = fmt.Sprintf("rl:%v", uid)
		} else {
			key = fmt.Sprintf("rl:%s", c.ClientIP())
		}

		ctx := c.Request.Context()
		now := time.Now().UnixMilli()
		windowMs := int64(1000) // 1 second window

		pipe := rdb.Pipeline()
		// Remove old entries outside the window
		pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now-windowMs))
		// Count entries in current window
		countCmd := pipe.ZCard(ctx, key)
		// Add current request
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
		// Set expiry
		pipe.Expire(ctx, key, time.Duration(windowMs)*time.Millisecond*2)

		_, err := pipe.Exec(ctx)
		if err != nil {
			// On Redis error, allow the request through
			c.Next()
			return
		}

		count := countCmd.Val()
		if count > int64(burst) {
			response.Err(c, http.StatusTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}

		c.Next()
	}
}
