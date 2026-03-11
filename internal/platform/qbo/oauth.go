package qbo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tonypk/aistarlight-go/internal/config"
)

// OAuthProvider handles QuickBooks Online OAuth 2.0 flows.
type OAuthProvider struct {
	cfg config.QBOConfig
}

// NewOAuthProvider creates a new OAuth provider.
func NewOAuthProvider(cfg config.QBOConfig) *OAuthProvider {
	return &OAuthProvider{cfg: cfg}
}

const (
	authURL  = "https://appcenter.intuit.com/connect/oauth2"
	tokenURL = "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"
)

// AuthURL generates the OAuth 2.0 authorization URL.
func (p *OAuthProvider) AuthURL(state string) string {
	params := url.Values{
		"client_id":     {p.cfg.ClientID},
		"redirect_uri":  {p.cfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {p.cfg.Scopes},
		"state":         {state},
	}
	return authURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *OAuthProvider) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {p.cfg.RedirectURL},
	}
	return p.tokenRequest(ctx, data)
}

// RefreshTokens refreshes an expired access token.
func (p *OAuthProvider) RefreshTokens(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return p.tokenRequest(ctx, data)
}

func (p *OAuthProvider) tokenRequest(ctx context.Context, data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(p.cfg.ClientID, p.cfg.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	return &tokenResp, nil
}
