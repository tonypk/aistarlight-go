// Package tools — audit tool implementations.
//
// Audit scan tools (scan_duplicates, scan_missing_receipts, scan_classification_issues)
// currently remain in ChatService (internal/service/audit_tools.go) and are handled
// by the ToolExecutor's ChatService fallback path. They will be migrated to the
// ToolRegistry in a follow-up when the underlying query methods are extracted from ChatService.
package tools

import "github.com/tonypk/aistarlight-go/internal/agent"

// RegisterAudit is a placeholder. Audit scan tools are currently served via ChatService fallback.
// See internal/service/audit_tools.go for implementations.
func RegisterAudit(r *agent.ToolRegistry) {
	// No-op: scan_duplicates, scan_missing_receipts, scan_classification_issues
	// are handled by ChatService fallback in ToolExecutor
}
