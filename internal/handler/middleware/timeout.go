package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout wraps the request context with a deadline.
// pgx and Redis operations respect context cancellation automatically.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		if ctx.Err() == context.DeadlineExceeded {
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
				"success": false,
				"error":   "request timeout",
			})
		}
	}
}
