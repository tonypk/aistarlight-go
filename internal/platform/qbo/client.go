package qbo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

// Client is a rate-limited QBO REST API v3 client.
type Client struct {
	baseURL  string
	limiter  *rate.Limiter
	http     *http.Client
}

// NewClient creates a new QBO API client with rate limiting.
func NewClient(baseURL string, ratePerMin int, maxConcur int) *Client {
	// Convert requests/minute to rate.Limit (per second).
	rps := float64(ratePerMin) / 60.0
	return &Client{
		baseURL: baseURL,
		limiter: rate.NewLimiter(rate.Limit(rps), maxConcur),
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Query executes a QBO query (e.g., "SELECT * FROM Account").
func (c *Client) Query(ctx context.Context, realmID, accessToken, query string) (*QueryResponse, error) {
	endpoint := fmt.Sprintf("%s/v3/company/%s/query", c.baseURL, realmID)

	params := url.Values{"query": {query}}
	fullURL := endpoint + "?" + params.Encode()

	body, err := c.doRequest(ctx, http.MethodGet, fullURL, accessToken)
	if err != nil {
		return nil, err
	}

	var qr QueryResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, fmt.Errorf("decode query response: %w", err)
	}
	return &qr, nil
}

// GetAccount fetches a single account by ID.
func (c *Client) GetAccount(ctx context.Context, realmID, accessToken, accountID string) (*QBOAccount, error) {
	endpoint := fmt.Sprintf("%s/v3/company/%s/account/%s", c.baseURL, realmID, accountID)

	body, err := c.doRequest(ctx, http.MethodGet, endpoint, accessToken)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Account QBOAccount `json:"Account"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode account response: %w", err)
	}
	return &resp.Account, nil
}

func (c *Client) doRequest(ctx context.Context, method, url, accessToken string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTokenExpired
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Fault.Error) > 0 {
			return nil, fmt.Errorf("QBO API error (%s): %s", errResp.Fault.Error[0].Code, errResp.Fault.Error[0].Message)
		}
		slog.Error("QBO API error", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("QBO API error (status %d)", resp.StatusCode)
	}

	return body, nil
}

// ErrTokenExpired indicates the access token is expired and needs refresh.
var ErrTokenExpired = fmt.Errorf("QBO access token expired")
