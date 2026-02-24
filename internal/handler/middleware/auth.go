package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
)

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	CompanyIDKey contextKey = "company_id"
	EmailKey     contextKey = "email"
	RoleKey      contextKey = "role"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID    uuid.UUID `json:"user_id"`
	CompanyID uuid.UUID `json:"company_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	// Legacy field — old Python tokens used tenant_id
	TenantID *uuid.UUID `json:"tenant_id,omitempty"`
}

type TokenRevokeChecker interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
}

type APIKeyResolver interface {
	ResolveAPIKey(ctx context.Context, key string) (*Claims, error)
}

func Auth(jwtSecret string, revokeChecker TokenRevokeChecker, apiKeyResolver APIKeyResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try API Key first
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			claims, err := apiKeyResolver.ResolveAPIKey(c.Request.Context(), apiKey)
			if err != nil {
				response.Unauthorized(c, "invalid API key")
				c.Abort()
				return
			}
			setClaimsToContext(c, claims)
			c.Next()
			return
		}

		// JWT Bearer token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		token, err := extractAndValidateJWT(authHeader, jwtSecret)
		if err != nil {
			response.Unauthorized(c, err.Error())
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			response.Unauthorized(c, "invalid token claims")
			c.Abort()
			return
		}

		// Handle legacy tenant_id → company_id mapping
		if claims.CompanyID == uuid.Nil && claims.TenantID != nil {
			claims.CompanyID = *claims.TenantID
		}

		// Check revocation
		if claims.ID != "" {
			revoked, err := revokeChecker.IsRevoked(c.Request.Context(), claims.ID)
			if err != nil || revoked {
				response.Unauthorized(c, "token has been revoked")
				c.Abort()
				return
			}
		}

		setClaimsToContext(c, claims)
		c.Next()
	}
}

func extractAndValidateJWT(authHeader, secret string) (*jwt.Token, error) {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, errors.New("invalid authorization header format")
	}

	token, err := jwt.ParseWithClaims(parts[1], &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, errors.New("invalid or expired token")
	}

	return token, nil
}

func setClaimsToContext(c *gin.Context, claims *Claims) {
	c.Set(string(UserIDKey), claims.UserID)
	c.Set(string(CompanyIDKey), claims.CompanyID)
	c.Set(string(EmailKey), claims.Email)
	c.Set(string(RoleKey), claims.Role)
}

// GetUserID extracts user_id from gin context.
func GetUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(string(UserIDKey))
	id, _ := v.(uuid.UUID)
	return id
}

// GetCompanyID extracts company_id from gin context.
func GetCompanyID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(string(CompanyIDKey))
	id, _ := v.(uuid.UUID)
	return id
}
