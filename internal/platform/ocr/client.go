package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tonypk/aistarlight-go/internal/service"
)

// Client calls the PaddleOCR microservice over HTTP.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates an OCR client pointing at the microservice URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// isRetryableError returns true for errors caused by OCR service restart (EOF, connection refused).
func isRetryableError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset")
}

// ExtractText sends an image to the OCR service and returns parsed text.
// Retries up to 3 times on transient errors (EOF / connection refused) to handle OCR restarts.
func (c *Client) ExtractText(ctx context.Context, imagePath string) (*service.OCRResult, error) {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait for OCR service to come back up after OOM restart.
			delay := time.Duration(attempt*5) * time.Second
			slog.Info("retrying OCR request", "attempt", attempt, "delay", delay, "last_error", lastErr)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		result, err := c.doExtractText(ctx, imagePath)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !isRetryableError(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

// doExtractText performs a single OCR request.
func (c *Client) doExtractText(ctx context.Context, imagePath string) (*service.OCRResult, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer func() { _ = f.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(imagePath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}
	_ = writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/ocr", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ocr request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ocr service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var ocrResp ocrResponse
	if err := json.NewDecoder(resp.Body).Decode(&ocrResp); err != nil {
		return nil, fmt.Errorf("decode ocr response: %w", err)
	}

	lines := make([]string, len(ocrResp.Lines))
	for i, l := range ocrResp.Lines {
		lines[i] = l.Text
	}

	return &service.OCRResult{
		Text:      ocrResp.Text,
		Lines:     lines,
		LineCount: ocrResp.LineCount,
	}, nil
}

type ocrResponse struct {
	Text      string     `json:"text"`
	Lines     []ocrLine  `json:"lines"`
	LineCount int        `json:"line_count"`
}

type ocrLine struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}
