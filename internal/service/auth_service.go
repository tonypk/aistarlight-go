package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrEmailTaken            = errors.New("email already registered")
	ErrInvalidCreds          = errors.New("invalid email or password")
	ErrUserInactive          = errors.New("user account is inactive")
	ErrTokenRevoked          = errors.New("token has been revoked")
	ErrInvalidToken          = errors.New("invalid or expired token")
	ErrCompanyNotFound       = errors.New("company not found")
	ErrNoAccess              = errors.New("no access to this company")
	ErrTelegramUsernameTaken = errors.New("telegram username already linked to another account")
	ErrSSONotConfigured      = errors.New("SSO integration not configured")
	ErrSSONoLink             = errors.New("no active integration link for this HR company")
)

type AuthService struct {
	q                *sqlc.Queries
	cfg              config.JWTConfig
	integrationSecret string
}

func NewAuthService(q *sqlc.Queries, cfg config.JWTConfig) *AuthService {
	return &AuthService{q: q, cfg: cfg}
}

// SetIntegrationSecret sets the shared secret for cross-app SSO tokens.
func (s *AuthService) SetIntegrationSecret(secret string) {
	s.integrationSecret = secret
}

type RegisterInput struct {
	Email        string
	Password     string
	FullName     string
	CompanyName  string
	Jurisdiction string
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*domain.User, error) {
	// Check email uniqueness
	existing, _ := s.q.GetUserByEmail(ctx, input.Email)
	if existing.ID != uuid.Nil {
		return nil, ErrEmailTaken
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create standalone company
	companyID := uuid.New()
	jurisdiction := input.Jurisdiction
	if jurisdiction == "" {
		jurisdiction = "PH"
	}
	_, err = s.q.CreateCompany(ctx, sqlc.CreateCompanyParams{
		ID:                companyID,
		CompanyName:       input.CompanyName,
		VatClassification: "vat_registered",
		FiscalYearEnd:     "12-31",
		Plan:              "free",
		Settings:          []byte("{}"),
		IsActive:          true,
		Jurisdiction:      jurisdiction,
	})
	if err != nil {
		return nil, fmt.Errorf("create company: %w", err)
	}

	// Create user
	userID := uuid.New()
	fullName := &input.FullName
	dbUser, err := s.q.CreateUser(ctx, sqlc.CreateUserParams{
		ID:             userID,
		Email:          input.Email,
		HashedPassword: string(hashed),
		FullName:       fullName,
		IsActive:       true,
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Add as company admin
	err = s.q.AddCompanyMember(ctx, sqlc.AddCompanyMemberParams{
		CompanyID: companyID,
		UserID:    userID,
		Role:      string(domain.CompanyRoleAdmin),
	})
	if err != nil {
		return nil, fmt.Errorf("add company member: %w", err)
	}

	user := &domain.User{
		ID:       dbUser.ID,
		Email:    dbUser.Email,
		FullName: dbUser.FullName,
		IsActive: dbUser.IsActive,
	}
	return user, nil
}

type CreateMemberInput struct {
	Email            string
	Password         string
	FullName         string
	TelegramUsername string
	CompanyID        uuid.UUID
	Role             string
}

type CreateMemberResult struct {
	User   *domain.User `json:"user"`
	APIKey string       `json:"api_key"`
}

func (s *AuthService) CreateMember(ctx context.Context, input CreateMemberInput) (*CreateMemberResult, error) {
	// Check email uniqueness
	existing, _ := s.q.GetUserByEmail(ctx, input.Email)
	if existing.ID != uuid.Nil {
		return nil, ErrEmailTaken
	}

	// Check telegram username uniqueness if provided
	var tgUsername *string
	if input.TelegramUsername != "" {
		existingTG, _ := s.q.GetUserByTelegramUsername(ctx, input.TelegramUsername)
		if existingTG.ID != uuid.Nil {
			return nil, ErrTelegramUsernameTaken
		}
		tgUsername = &input.TelegramUsername
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	userID := uuid.New()
	fullName := &input.FullName
	dbUser, err := s.q.CreateUser(ctx, sqlc.CreateUserParams{
		ID:               userID,
		Email:            input.Email,
		HashedPassword:   string(hashed),
		FullName:         fullName,
		IsActive:         true,
		TelegramUsername: tgUsername,
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Add as company member with specified role (default: member)
	role := input.Role
	if role == "" {
		role = string(domain.CompanyRoleMember)
	}
	err = s.q.AddCompanyMember(ctx, sqlc.AddCompanyMemberParams{
		CompanyID: input.CompanyID,
		UserID:    userID,
		Role:      role,
	})
	if err != nil {
		return nil, fmt.Errorf("add company member: %w", err)
	}

	// Generate API key
	apiKey, err := s.GenerateAPIKey(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	return &CreateMemberResult{
		User: &domain.User{
			ID:       dbUser.ID,
			Email:    dbUser.Email,
			FullName: dbUser.FullName,
			IsActive: dbUser.IsActive,
		},
		APIKey: apiKey,
	}, nil
}

// GetByEmail returns a user by their email address.
func (s *AuthService) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	dbUser, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	return &domain.User{
		ID:       dbUser.ID,
		Email:    dbUser.Email,
		FullName: dbUser.FullName,
		IsActive: dbUser.IsActive,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, *uuid.UUID, string, error) {
	dbUser, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, nil, "", ErrInvalidCreds
	}

	if !dbUser.IsActive {
		return nil, nil, "", ErrUserInactive
	}

	if err := bcrypt.CompareHashAndPassword([]byte(dbUser.HashedPassword), []byte(password)); err != nil {
		return nil, nil, "", ErrInvalidCreds
	}

	// Get the first company the user has access to
	companies, err := s.q.ListCompaniesByUser(ctx, sqlc.ListCompaniesByUserParams{
		UserID: dbUser.ID,
		Limit:  1,
		Offset: 0,
	})
	if err != nil || len(companies) == 0 {
		return nil, nil, "", ErrCompanyNotFound
	}

	companyID := companies[0].ID
	role := string(domain.CompanyRoleAdmin) // default

	effectiveRole, err := s.q.GetEffectiveRole(ctx, sqlc.GetEffectiveRoleParams{
		UserID:    dbUser.ID,
		CompanyID: companyID,
	})
	if err == nil && effectiveRole != nil {
		if r, ok := effectiveRole.(string); ok {
			role = r
		}
	}

	// Get company jurisdiction
	company, err := s.q.GetCompanyByID(ctx, companyID)
	if err != nil {
		return nil, nil, "", ErrCompanyNotFound
	}

	tokens, err := s.generateTokenPair(dbUser.ID, companyID, dbUser.Email, role, company.Jurisdiction)
	if err != nil {
		return nil, nil, "", err
	}

	return tokens, &companyID, company.Jurisdiction, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Check revocation
	if claims.ID != "" {
		result, err := s.q.IsTokenRevoked(ctx, claims.ID)
		if err == nil && result {
			return nil, ErrTokenRevoked
		}
	}

	// Revoke old refresh token
	if claims.ID != "" {
		_ = s.q.CreateRevokedToken(ctx, sqlc.CreateRevokedTokenParams{
			ID:        uuid.New(),
			Jti:       claims.ID,
			UserID:    claims.UserID,
			RevokedAt: time.Now(),
			ExpiresAt: claims.ExpiresAt.Time,
		})
	}

	// Handle legacy tenant_id
	companyID := claims.CompanyID
	if companyID == uuid.Nil && claims.TenantID != nil {
		companyID = *claims.TenantID
	}

	jurisdiction := claims.Jurisdiction
	if jurisdiction == "" {
		jurisdiction = "PH"
	}

	tokens, err := s.generateTokenPair(claims.UserID, companyID, claims.Email, claims.Role, jurisdiction)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return ErrInvalidToken
	}

	if claims.ID != "" {
		return s.q.CreateRevokedToken(ctx, sqlc.CreateRevokedTokenParams{
			ID:        uuid.New(),
			Jti:       claims.ID,
			UserID:    claims.UserID,
			RevokedAt: time.Now(),
			ExpiresAt: claims.ExpiresAt.Time,
		})
	}

	return nil
}

func (s *AuthService) SwitchCompany(ctx context.Context, userID, companyID uuid.UUID) (*TokenPair, string, error) {
	// Verify company exists
	company, err := s.q.GetCompanyByID(ctx, companyID)
	if err != nil {
		return nil, "", ErrCompanyNotFound
	}

	// Verify user has access
	effectiveRole, err := s.q.GetEffectiveRole(ctx, sqlc.GetEffectiveRoleParams{
		UserID:    userID,
		CompanyID: companyID,
	})
	if err != nil || effectiveRole == nil {
		return nil, "", ErrNoAccess
	}

	role, ok := effectiveRole.(string)
	if !ok {
		return nil, "", ErrNoAccess
	}

	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", fmt.Errorf("get user: %w", err)
	}

	tokens, err := s.generateTokenPair(userID, companyID, user.Email, role, company.Jurisdiction)
	if err != nil {
		return nil, "", err
	}

	return tokens, company.Jurisdiction, nil
}

func (s *AuthService) GenerateAPIKey(ctx context.Context, userID uuid.UUID) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	apiKey := hex.EncodeToString(key)

	err := s.q.SetAPIKey(ctx, sqlc.SetAPIKeyParams{
		ID:     userID,
		ApiKey: &apiKey,
	})
	if err != nil {
		return "", fmt.Errorf("save api key: %w", err)
	}

	return apiKey, nil
}

func (s *AuthService) ResolveAPIKey(ctx context.Context, key string) (*middleware.Claims, error) {
	dbUser, err := s.q.GetUserByAPIKey(ctx, &key)
	if err != nil {
		return nil, errors.New("invalid API key")
	}

	// Get first company
	companies, err := s.q.ListCompaniesByUser(ctx, sqlc.ListCompaniesByUserParams{
		UserID: dbUser.ID,
		Limit:  1,
		Offset: 0,
	})
	if err != nil || len(companies) == 0 {
		return nil, errors.New("no company found")
	}

	return &middleware.Claims{
		UserID:       dbUser.ID,
		CompanyID:    companies[0].ID,
		Email:        dbUser.Email,
		Role:         string(domain.CompanyRoleAdmin),
		Jurisdiction: companies[0].Jurisdiction,
	}, nil
}

// IsRevoked implements middleware.TokenRevokeChecker.
func (s *AuthService) IsRevoked(ctx context.Context, jti string) (bool, error) {
	return s.q.IsTokenRevoked(ctx, jti)
}

// CrossAppClaims represents the claims from an AIGoNHR cross-app SSO token.
type CrossAppClaims struct {
	jwt.RegisteredClaims
	HRCompanyID int64  `json:"hr_company_id"`
	HRUserID    int64  `json:"hr_user_id"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
}

// SSOLogin validates a cross-app JWT from AIGoNHR and returns local tokens.
// It looks up the integration_sources by remote_company_id, finds or creates
// a local user by email, and issues a standard AIStarlight token pair.
func (s *AuthService) SSOLogin(ctx context.Context, ssoToken string) (*TokenPair, *uuid.UUID, string, error) {
	if s.integrationSecret == "" {
		return nil, nil, "", ErrSSONotConfigured
	}

	// Validate the cross-app JWT
	token, err := jwt.ParseWithClaims(ssoToken, &CrossAppClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.integrationSecret), nil
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid SSO token: %w", err)
	}

	claims, ok := token.Claims.(*CrossAppClaims)
	if !ok || !token.Valid {
		return nil, nil, "", ErrInvalidToken
	}

	// Look up integration source by remote company ID
	remoteID := fmt.Sprintf("%d", claims.HRCompanyID)
	source, err := s.q.GetIntegrationSourceByRemoteCompany(ctx, sqlc.GetIntegrationSourceByRemoteCompanyParams{
		SourceSystem:    "aigonhr",
		RemoteCompanyID: remoteID,
	})
	if err != nil {
		return nil, nil, "", ErrSSONoLink
	}

	companyID := source.CompanyID

	// Get company details for jurisdiction
	company, err := s.q.GetCompanyByID(ctx, companyID)
	if err != nil {
		return nil, nil, "", ErrCompanyNotFound
	}

	// Find or create user by email
	var userID uuid.UUID
	var userEmail string

	dbUser, err := s.q.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		// User doesn't exist — auto-create
		fullName := fmt.Sprintf("%s %s", claims.FirstName, claims.LastName)
		userID = uuid.New()
		randomPW := make([]byte, 32)
		_, _ = rand.Read(randomPW)
		hashed, hashErr := bcrypt.GenerateFromPassword(randomPW, bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, nil, "", fmt.Errorf("hash password: %w", hashErr)
		}

		created, createErr := s.q.CreateUser(ctx, sqlc.CreateUserParams{
			ID:             userID,
			Email:          claims.Email,
			HashedPassword: string(hashed),
			FullName:       &fullName,
			IsActive:       true,
		})
		if createErr != nil {
			return nil, nil, "", fmt.Errorf("create SSO user: %w", createErr)
		}
		userEmail = created.Email

		// Map HR role to local role
		localRole := string(domain.CompanyRoleMember)
		if claims.Role == "admin" || claims.Role == "super_admin" {
			localRole = string(domain.CompanyRoleAdmin)
		}

		_ = s.q.AddCompanyMember(ctx, sqlc.AddCompanyMemberParams{
			CompanyID: companyID,
			UserID:    userID,
			Role:      localRole,
		})
	} else {
		userID = dbUser.ID
		userEmail = dbUser.Email
	}

	// Resolve effective role
	role := string(domain.CompanyRoleMember)
	effectiveRole, err := s.q.GetEffectiveRole(ctx, sqlc.GetEffectiveRoleParams{
		UserID:    userID,
		CompanyID: companyID,
	})
	if err == nil && effectiveRole != nil {
		if r, ok := effectiveRole.(string); ok {
			role = r
		}
	}

	tokens, err := s.generateTokenPair(userID, companyID, userEmail, role, company.Jurisdiction)
	if err != nil {
		return nil, nil, "", err
	}

	return tokens, &companyID, company.Jurisdiction, nil
}

func (s *AuthService) generateTokenPair(userID, companyID uuid.UUID, email, role, jurisdiction string) (*TokenPair, error) {
	now := time.Now()
	jti := uuid.New().String()

	if jurisdiction == "" {
		jurisdiction = "PH"
	}

	accessClaims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.cfg.ExpiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
		UserID:       userID,
		CompanyID:    companyID,
		Email:        email,
		Role:         role,
		Jurisdiction: jurisdiction,
		// Include tenant_id for backward compat with Vue frontend
		TenantID: &companyID,
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshJTI := uuid.New().String()
	refreshClaims := middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.cfg.RefreshExpiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        refreshJTI,
		},
		UserID:       userID,
		CompanyID:    companyID,
		Email:        email,
		Role:         role,
		Jurisdiction: jurisdiction,
		TenantID:     &companyID,
	}

	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "bearer",
	}, nil
}

func (s *AuthService) parseToken(tokenStr string) (*middleware.Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &middleware.Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.Secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*middleware.Claims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	return claims, nil
}
