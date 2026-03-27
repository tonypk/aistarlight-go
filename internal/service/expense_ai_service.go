package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ExpenseAIService handles AI-powered expense processing:
// OCR receipt extraction, risk scoring, and merchant classification.
type ExpenseAIService struct {
	q     *sqlc.Queries
	pool  *pgxpool.Pool
	redis *redis.Client
	glSvc ExpenseGLCreator
}

// NewExpenseAIService creates a new ExpenseAIService.
func NewExpenseAIService(q *sqlc.Queries, pool *pgxpool.Pool, redis *redis.Client) *ExpenseAIService {
	return &ExpenseAIService{q: q, pool: pool, redis: redis}
}

// SetGLService injects the GL service after construction (breaks circular dependency).
func (s *ExpenseAIService) SetGLService(glSvc ExpenseGLCreator) {
	s.glSvc = glSvc
}

// ExpenseOCRResult holds structured data extracted from a receipt image.
type ExpenseOCRResult struct {
	MerchantName string  `json:"merchant_name"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Date         string  `json:"date"`
	Description  string  `json:"description"`
	Confidence   float64 `json:"confidence"`
}

// ExtractReceipt extracts data from a receipt image.
// Currently a stub that returns nil — real implementation would use Claude Vision API.
// Receipt processing should never block the upload flow.
func (s *ExpenseAIService) ExtractReceipt(ctx context.Context, imageData []byte) (*ExpenseOCRResult, error) {
	// TODO: Implement Claude Vision API call
	// For now, return nil (OCR is optional, not blocking)
	return nil, nil
}

// EvaluateReport performs risk scoring on a submitted expense report and
// auto-approves low-risk reports or flags them for human review.
func (s *ExpenseAIService) EvaluateReport(ctx context.Context, reportID, companyID uuid.UUID) error {
	// 1. Load report
	report, err := s.q.GetExpenseReportByID(ctx, sqlc.GetExpenseReportByIDParams{
		ID:        reportID,
		CompanyID: companyID,
	})
	if err != nil {
		return fmt.Errorf("get report: %w", err)
	}

	// 2. Load items
	items, err := s.q.ListExpenseItemsByReport(ctx, reportID)
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}

	// 3. Load company policies
	policies, err := s.q.ListExpensePolicies(ctx, companyID)
	if err != nil {
		policies = nil // proceed without policies
	}

	// Build policy lookup by category
	policyMap := make(map[string]sqlc.ExpensePolicy)
	for _, p := range policies {
		policyMap[p.Category] = p
	}

	// 4. Score each item, aggregate
	totalScore := 10 // base score
	canAutoApprove := true

	for _, item := range items {
		itemAmount := numericToDecimalVal(item.Amount)
		policy, hasPolicy := policyMap[item.Category]

		// Check: exceeds policy max
		if hasPolicy {
			policyMax := numericToPtrDecimal(policy.MaxAmount)
			if policyMax != nil && itemAmount.GreaterThan(*policyMax) {
				totalScore += 15
				canAutoApprove = false
			}

			// Check: missing receipt above threshold
			receiptThreshold := numericToPtrDecimal(policy.RequiresReceiptAbove)
			if receiptThreshold != nil && itemAmount.GreaterThan(*receiptThreshold) {
				if item.ReceiptUrl == nil || *item.ReceiptUrl == "" {
					totalScore += 20
					canAutoApprove = false
				}
			}

			// Check AI auto-approve flag
			if !policy.AiAutoApprove {
				canAutoApprove = false
			}
		} else {
			canAutoApprove = false // no policy = can't auto-approve
		}

		// Check: weekend transaction
		if item.TransactionDate.Valid {
			wd := item.TransactionDate.Time.Weekday()
			if wd == time.Saturday || wd == time.Sunday {
				totalScore += 10
			}
		}

		// Check: duplicates
		if item.MerchantName != nil && *item.MerchantName != "" {
			dupes, err := s.q.FindDuplicateExpenseItems(ctx, sqlc.FindDuplicateExpenseItemsParams{
				CompanyID:       companyID,
				SubmitterUserID: report.SubmitterUserID,
				ExpenseReportID: reportID,
				Amount:          item.Amount,
				MerchantName:    item.MerchantName,
				Column6:         item.TransactionDate,
			})
			if err == nil && len(dupes) > 0 {
				totalScore += 10 * len(dupes)
			}
		}
	}

	// Cap at 100
	if totalScore > 100 {
		totalScore = 100
	}

	// 5. Determine decision
	var decision string
	var newStatus string
	if totalScore < 30 && canAutoApprove {
		decision = string(domain.AIDecisionAutoApproved)
		newStatus = string(domain.ExpenseStatusApproved)
	} else if totalScore >= 70 {
		decision = string(domain.AIDecisionHighRisk)
		newStatus = string(domain.ExpenseStatusPendingApproval)
	} else {
		decision = string(domain.AIDecisionNeedsReview)
		newStatus = string(domain.ExpenseStatusPendingApproval)
	}

	reason := fmt.Sprintf("Risk score: %d/100. Items: %d.", totalScore, len(items))

	// 6. Update report
	riskScore := int32(totalScore)
	err = s.q.UpdateExpenseReportAIReview(ctx, sqlc.UpdateExpenseReportAIReviewParams{
		ID:               reportID,
		CompanyID:        companyID,
		Status:           newStatus,
		AiRiskScore:      &riskScore,
		AiDecision:       &decision,
		AiDecisionReason: &reason,
	})
	if err != nil {
		return fmt.Errorf("update AI review: %w", err)
	}

	// 7. If auto-approved, generate GL accrual entry
	if decision == string(domain.AIDecisionAutoApproved) && s.glSvc != nil {
		domainReport := toExpenseReport(report)
		domainReport.Status = domain.ExpenseStatusApproved
		domainItems := make([]domain.ExpenseItem, len(items))
		for i, item := range items {
			domainItems[i] = *toExpenseItem(item)
		}
		journalID, err := s.glSvc.CreateAccrualEntry(ctx, domainReport, domainItems)
		if err == nil && journalID != nil {
			// Link journal entry to report — ignore error (best-effort)
			_ = s.q.UpdateExpenseReportApprove(ctx, sqlc.UpdateExpenseReportApproveParams{
				ID:                    reportID,
				CompanyID:             companyID,
				ApproverUserID:        pgtype.UUID{}, // AI approved, no human approver
				AccrualJournalEntryID: pgtype.UUID{Bytes: *journalID, Valid: true},
			})
		}
	}

	// 8. Write audit log — ignore error (best-effort)
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"risk_score": totalScore,
		"decision":   decision,
		"reason":     reason,
	})
	_, _ = s.q.CreateExpenseAuditLog(ctx, sqlc.CreateExpenseAuditLogParams{
		ID:              uuid.New(),
		ExpenseReportID: pgtype.UUID{Bytes: reportID, Valid: true},
		Action:          domain.AuditActionAIReview,
		ActorUserID:     pgtype.UUID{}, // AI actor
		ActorType:       "ai",
		Details:         detailsJSON,
	})

	return nil
}

// ClassifyMerchant returns the most-used expense category for a merchant,
// using Redis for caching and the DB for historical lookup.
func (s *ExpenseAIService) ClassifyMerchant(ctx context.Context, companyID uuid.UUID, merchantName string) (string, float64, error) {
	// Check Redis cache
	cacheKey := fmt.Sprintf("expense:merchants:%s:%s", companyID.String(), merchantName)
	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			return cached, 0.9, nil
		}
	}

	// Check DB history
	row, err := s.q.GetMerchantCategory(ctx, sqlc.GetMerchantCategoryParams{
		CompanyID:    companyID,
		MerchantName: &merchantName,
	})
	if err == nil {
		// Cache in Redis
		if s.redis != nil {
			s.redis.Set(ctx, cacheKey, row.Category, 7*24*time.Hour)
		}
		return row.Category, 0.85, nil
	}

	// Default to "other"
	return domain.CategoryOther, 0.5, nil
}
