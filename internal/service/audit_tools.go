package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// executeScanDuplicates finds suspected duplicate transactions for a given month.
func (s *ChatService) executeScanDuplicates(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	month := toString(args["month"])
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	txns, err := s.listMonthTransactions(ctx, companyID, month)
	if err != nil {
		return jsonError("failed to query transactions: " + err.Error())
	}
	if len(txns) == 0 {
		return jsonResult(map[string]interface{}{"groups": []interface{}{}, "message": fmt.Sprintf("No transactions found for %s.", month)})
	}

	groups := findDuplicateGroups(txns)

	out := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		txList := make([]map[string]interface{}, 0, len(g.txns))
		for _, t := range g.txns {
			entry := map[string]interface{}{
				"ref":       fmt.Sprintf("TXN-%d", t.refNumber),
				"date":      t.date.Format("2006-01-02"),
				"submitter": t.submitter,
			}
			txList = append(txList, entry)
		}
		out = append(out, map[string]interface{}{
			"amount":       g.amount,
			"description":  g.desc,
			"transactions": txList,
			"reason":       g.reason,
		})
	}

	return jsonResult(map[string]interface{}{
		"groups": out,
		"count":  len(out),
		"month":  month,
	})
}

// executeScanMissingReceipts finds high-value transactions not backed by receipts.
func (s *ChatService) executeScanMissingReceipts(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	month := toString(args["month"])
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	threshold := 1000.0
	if v, ok := args["threshold"].(float64); ok && v > 0 {
		threshold = v
	}

	txns, err := s.listMonthTransactions(ctx, companyID, month)
	if err != nil {
		return jsonError("failed to query transactions: " + err.Error())
	}

	var missing []map[string]interface{}
	for _, t := range txns {
		if t.SourceType == "receipt" {
			continue
		}
		f, fErr := t.Amount.Float64Value()
		if fErr != nil || !f.Valid {
			continue
		}
		amt := math.Abs(f.Float64)
		if amt < threshold {
			continue
		}

		ref := int32(0)
		if t.RefNumber != nil {
			ref = *t.RefNumber
		}
		missing = append(missing, map[string]interface{}{
			"ref":         fmt.Sprintf("TXN-%d", ref),
			"date":        t.Date.Time.Format("2006-01-02"),
			"amount":      f.Float64,
			"description": descOrFallback(t.Description),
			"submitter":   t.SubmittedByName,
		})
	}

	if missing == nil {
		missing = []map[string]interface{}{}
	}

	return jsonResult(map[string]interface{}{
		"count":        len(missing),
		"transactions": missing,
		"month":        month,
		"threshold":    threshold,
	})
}

// executeScanClassificationIssues finds transactions with low confidence or default classifications.
func (s *ChatService) executeScanClassificationIssues(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	month := toString(args["month"])
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	txns, err := s.listMonthTransactions(ctx, companyID, month)
	if err != nil {
		return jsonError("failed to query transactions: " + err.Error())
	}

	var issues []map[string]interface{}
	for _, t := range txns {
		conf, cErr := t.Confidence.Float64Value()
		if cErr != nil || !conf.Valid {
			continue
		}

		isLowConf := conf.Float64 < 0.6
		isDefault := t.ClassificationSource == "default"
		if !isLowConf && !isDefault {
			continue
		}

		ref := int32(0)
		if t.RefNumber != nil {
			ref = *t.RefNumber
		}
		amt, _ := t.Amount.Float64Value()
		issues = append(issues, map[string]interface{}{
			"ref":         fmt.Sprintf("TXN-%d", ref),
			"date":        t.Date.Time.Format("2006-01-02"),
			"amount":      amt.Float64,
			"description": descOrFallback(t.Description),
			"category":    t.Category,
			"confidence":  conf.Float64,
			"source":      t.ClassificationSource,
		})
	}

	if issues == nil {
		issues = []map[string]interface{}{}
	}

	return jsonResult(map[string]interface{}{
		"count":        len(issues),
		"transactions": issues,
		"month":        month,
	})
}

// listMonthTransactions is a helper that fetches all transactions for a company in a given YYYY-MM month.
func (s *ChatService) listMonthTransactions(ctx context.Context, companyID uuid.UUID, month string) ([]sqlc.ListTransactionsWithSubmitterRow, error) {
	startDate, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, fmt.Errorf("invalid month format %q, expected YYYY-MM", month)
	}
	endDate := startDate.AddDate(0, 1, -1)

	txns, err := s.q.ListTransactionsWithSubmitter(ctx, sqlc.ListTransactionsWithSubmitterParams{
		CompanyID: companyID,
		Date:      pgtype.Date{Time: startDate, Valid: true},
		Date_2:    pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		slog.Error("listMonthTransactions query failed", "error", err, "month", month)
		return nil, err
	}
	return txns, nil
}

// --- Duplicate detection (rewritten from bot/audit.go to avoid cross-package dependency) ---

type auditDupGroup struct {
	amount float64
	desc   string
	txns   []auditDupTxn
	reason string
}

type auditDupTxn struct {
	refNumber int32
	date      time.Time
	submitter string
	desc      string
}

func findDuplicateGroups(txns []sqlc.ListTransactionsWithSubmitterRow) []auditDupGroup {
	type amountKey int64
	byAmount := map[amountKey][]sqlc.ListTransactionsWithSubmitterRow{}
	for _, t := range txns {
		f, err := t.Amount.Float64Value()
		if err != nil || !f.Valid {
			continue
		}
		key := amountKey(math.Round(f.Float64 * 100))
		byAmount[key] = append(byAmount[key], t)
	}

	var groups []auditDupGroup
	for _, bucket := range byAmount {
		if len(bucket) < 2 {
			continue
		}
		used := make([]bool, len(bucket))
		for i := 0; i < len(bucket); i++ {
			if used[i] {
				continue
			}
			cluster := []int{i}
			for j := i + 1; j < len(bucket); j++ {
				if used[j] {
					continue
				}
				if isSuspiciousPair(bucket[i], bucket[j]) {
					cluster = append(cluster, j)
				}
			}
			if len(cluster) < 2 {
				continue
			}
			for _, idx := range cluster {
				used[idx] = true
			}

			g := auditDupGroup{}
			f, _ := bucket[cluster[0]].Amount.Float64Value()
			g.amount = f.Float64
			g.desc = descOrFallback(bucket[cluster[0]].Description)

			var minDate, maxDate time.Time
			for _, idx := range cluster {
				t := bucket[idx]
				at := auditDupTxn{
					submitter: t.SubmittedByName,
					desc:      descOrFallback(t.Description),
				}
				if t.RefNumber != nil {
					at.refNumber = *t.RefNumber
				}
				if t.Date.Valid {
					at.date = t.Date.Time
					if minDate.IsZero() || t.Date.Time.Before(minDate) {
						minDate = t.Date.Time
					}
					if maxDate.IsZero() || t.Date.Time.After(maxDate) {
						maxDate = t.Date.Time
					}
				}
				g.txns = append(g.txns, at)
			}

			days := int(maxDate.Sub(minDate).Hours() / 24)
			switch {
			case days == 0:
				g.reason = "same day — likely duplicate"
			case days == 1:
				g.reason = "1 day apart"
			default:
				g.reason = fmt.Sprintf("%d days apart", days)
			}
			groups = append(groups, g)
		}
	}
	return groups
}

func isSuspiciousPair(a, b sqlc.ListTransactionsWithSubmitterRow) bool {
	if !a.Date.Valid || !b.Date.Valid {
		return false
	}
	daysDiff := int(math.Abs(a.Date.Time.Sub(b.Date.Time).Hours() / 24))
	if daysDiff > 3 {
		return false
	}
	descA := strings.ToLower(strings.TrimSpace(descOrFallback(a.Description)))
	descB := strings.ToLower(strings.TrimSpace(descOrFallback(b.Description)))
	if descA == descB {
		return true
	}
	if descA != "" && descB != "" {
		if strings.Contains(descA, descB) || strings.Contains(descB, descA) {
			return true
		}
	}
	return false
}

func descOrFallback(s *string) string {
	if s != nil && *s != "" {
		return *s
	}
	return "(no description)"
}

func jsonResult(data map[string]interface{}) string {
	result, _ := json.Marshal(data)
	return string(result)
}
