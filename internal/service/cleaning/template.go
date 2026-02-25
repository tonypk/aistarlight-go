package cleaning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// TemplateService handles saving and matching cleaning templates.
type TemplateService struct {
	q *sqlc.Queries
}

// NewTemplateService creates a template service.
func NewTemplateService(q *sqlc.Queries) *TemplateService {
	return &TemplateService{q: q}
}

// MatchTemplate looks for a saved template matching the given columns.
// Returns the AI result if found, nil otherwise.
func (ts *TemplateService) MatchTemplate(ctx context.Context, companyID uuid.UUID, columns []string) (*AISemanticResult, error) {
	if ts == nil || ts.q == nil {
		return nil, nil
	}

	hash := HeaderSignatureHash(columns)
	tmpl, err := ts.q.GetCleaningTemplateByHash(ctx, sqlc.GetCleaningTemplateByHashParams{
		CompanyID:     companyID,
		SignatureHash: hash,
	})
	if err != nil {
		// Not found is not an error
		return nil, nil
	}

	// Parse the stored header signature for Jaccard validation
	var storedSig []string
	if err := json.Unmarshal(tmpl.HeaderSignature, &storedSig); err != nil {
		slog.Warn("failed to parse stored header signature", "error", err)
		return nil, nil
	}

	// Verify with Jaccard similarity (defense against hash collisions)
	currentSig := HeaderSignature(columns)
	similarity := JaccardSimilarity(currentSig, storedSig)
	if similarity < 0.85 {
		slog.Warn("template hash matched but Jaccard too low",
			"similarity", similarity,
			"stored_sig", storedSig,
			"current_sig", currentSig,
		)
		return nil, nil
	}

	// Parse the stored AI result
	var result AISemanticResult
	if err := json.Unmarshal(tmpl.AiResult, &result); err != nil {
		slog.Warn("failed to parse stored AI result", "error", err)
		return nil, nil
	}

	// Increment hit count
	if err := ts.q.IncrementCleaningTemplateHitCount(ctx, tmpl.ID); err != nil {
		slog.Warn("failed to increment template hit count", "error", err)
	}

	slog.Info("cleaning template matched",
		"template_id", tmpl.ID,
		"hit_count", tmpl.HitCount+1,
		"similarity", similarity,
	)

	return &result, nil
}

// SaveTemplate stores an AI result as a reusable template.
func (ts *TemplateService) SaveTemplate(ctx context.Context, companyID uuid.UUID, columns []string, result *AISemanticResult) error {
	if ts == nil || ts.q == nil || result == nil {
		return nil
	}

	hash := HeaderSignatureHash(columns)
	sig := HeaderSignature(columns)

	sigJSON, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("marshal header signature: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal AI result: %w", err)
	}

	_, err = ts.q.UpsertCleaningTemplate(ctx, sqlc.UpsertCleaningTemplateParams{
		CompanyID:       companyID,
		SignatureHash:   hash,
		HeaderSignature: sigJSON,
		AiResult:        resultJSON,
	})
	if err != nil {
		return fmt.Errorf("upsert cleaning template: %w", err)
	}

	slog.Info("cleaning template saved",
		"company_id", companyID,
		"hash", hash[:16]+"...",
		"table_type", result.TableType,
		"columns", len(result.Columns),
	)

	return nil
}
