package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
)

// AIHeaderResult is the structured result from AI header detection.
type AIHeaderResult struct {
	Columns      []string `json:"columns"`
	DataStartRow int      `json:"data_start_row"`
	DataEndRow   int      `json:"data_end_row"` // -1 means "until the end"
}

const headerDetectionPrompt = `You are a spreadsheet data boundary detection expert for Philippine BIR tax forms and financial documents.

Given the rows of a spreadsheet, identify:
1. The column header names (if headers span multiple rows, merge them into single descriptive names)
2. Which row number (0-indexed) the actual transaction data STARTS
3. Which row number (0-indexed) the actual transaction data ENDS (the last real data row, INCLUSIVE)

Rules for HEADER detection (top of data):
- Skip title rows (e.g. "QUARTERLY SUMMARY LIST OF PURCHASES"), metadata, and form labels
- Skip numbering rows like "1, 2, 3, 4" or "(1), (2), (3)"
- Merge multi-row headers into descriptive names. Example:
  Row A: ["TAXPAYER", "", "AMOUNT OF"]
  Row B: ["IDENTIFICATION", "", "GROSS PURCHASE"]
  Row C: ["NUMBER", "", ""]
  → Merged: ["TAXPAYER IDENTIFICATION NUMBER", "", "AMOUNT OF GROSS PURCHASE"]
- If a column has no header text at all, use "Column_N" where N is the 1-based position
- Remove trailing empty columns

Rules for FOOTER detection (bottom of data):
- data_end_row should be the LAST row that contains a real individual transaction/record
- Exclude these footer rows from the data range:
  * Summary/total rows: "Grand Total", "Total", "Subtotal", "Total Sales", "Total Purchases", etc.
  * Aggregate rows: any row that sums up or aggregates the data above
  * Empty separator rows at the bottom
  * Notes, remarks, signatures, "Prepared by", "Certified correct", etc.
  * Any metadata or form footer that is not an individual transaction
- If there are NO footer rows (data goes to the end), set data_end_row to the last row index

Return ONLY valid JSON with this exact structure:
{"columns": ["Col1", "Col2", ...], "data_start_row": <int>, "data_end_row": <int>}`

// DetectHeadersWithAI uses OpenAI to detect column headers and data boundaries
// from raw spreadsheet rows. Returns nil if AI client is not available.
func DetectHeadersWithAI(ctx context.Context, ai *openai.Client, rows [][]string) (*AIHeaderResult, error) {
	if ai == nil {
		return nil, fmt.Errorf("AI client not available")
	}

	// Send the first 30 rows (for header detection) + the last 20 rows (for footer detection).
	// For small sheets, just send everything.
	var sb strings.Builder
	fmt.Fprintf(&sb, "Spreadsheet has %d total rows.\n\n", len(rows))

	headLimit := 30
	if headLimit > len(rows) {
		headLimit = len(rows)
	}
	tailStart := len(rows) - 20
	if tailStart < headLimit {
		// Small sheet — just send all rows
		sb.WriteString("ALL ROWS:\n\n")
		for i := 0; i < len(rows); i++ {
			sb.WriteString(fmt.Sprintf("Row %d: %s\n", i, formatRowForAI(rows[i])))
		}
	} else {
		sb.WriteString(fmt.Sprintf("FIRST %d ROWS (for header detection):\n\n", headLimit))
		for i := 0; i < headLimit; i++ {
			sb.WriteString(fmt.Sprintf("Row %d: %s\n", i, formatRowForAI(rows[i])))
		}
		sb.WriteString(fmt.Sprintf("\n... (rows %d-%d omitted) ...\n\n", headLimit, tailStart-1))
		sb.WriteString(fmt.Sprintf("LAST %d ROWS (for footer/total detection):\n\n", len(rows)-tailStart))
		for i := tailStart; i < len(rows); i++ {
			sb.WriteString(fmt.Sprintf("Row %d: %s\n", i, formatRowForAI(rows[i])))
		}
	}

	resp, err := ai.ChatCompletion(ctx, []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: headerDetectionPrompt},
		{Role: oai.ChatMessageRoleUser, Content: sb.String()},
	},
		openai.WithTemperature(0),
		openai.WithMaxTokens(2000),
		openai.WithJSONResponse(),
		openai.WithModel("gpt-4.1-mini"),
	)
	if err != nil {
		return nil, fmt.Errorf("AI header detection API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("AI returned empty response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)

	var result AIHeaderResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response JSON: %w (raw: %.500s)", err, content)
	}

	if len(result.Columns) == 0 {
		return nil, fmt.Errorf("AI returned zero columns")
	}

	if result.DataStartRow < 0 || result.DataStartRow >= len(rows) {
		return nil, fmt.Errorf("AI returned invalid data_start_row: %d (total rows: %d)", result.DataStartRow, len(rows))
	}

	// Validate/default data_end_row
	if result.DataEndRow <= 0 || result.DataEndRow >= len(rows) {
		result.DataEndRow = len(rows) - 1
	}
	if result.DataEndRow < result.DataStartRow {
		result.DataEndRow = len(rows) - 1
	}

	// Clean up: trim whitespace and replace empty trailing columns.
	for i := range result.Columns {
		result.Columns[i] = strings.TrimSpace(result.Columns[i])
	}
	// Remove trailing empty columns.
	for len(result.Columns) > 0 && result.Columns[len(result.Columns)-1] == "" {
		result.Columns = result.Columns[:len(result.Columns)-1]
	}
	// Replace remaining empty names with positional labels.
	for i, c := range result.Columns {
		if c == "" {
			result.Columns[i] = fmt.Sprintf("Column_%d", i+1)
		}
	}

	slog.Info("AI data boundary detection succeeded",
		"num_columns", len(result.Columns),
		"data_start_row", result.DataStartRow,
		"data_end_row", result.DataEndRow,
		"total_rows", len(rows),
		"first_cols", truncateSlice(result.Columns, 5),
	)

	return &result, nil
}

// formatRowForAI formats a row as a JSON-like array string for the AI prompt.
func formatRowForAI(row []string) string {
	parts := make([]string, len(row))
	for i, cell := range row {
		v := strings.TrimSpace(cell)
		if v == "" {
			parts[i] = `""`
		} else {
			parts[i] = fmt.Sprintf("%q", v)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// truncateSlice returns the first n elements of a string slice for logging.
func truncateSlice(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
