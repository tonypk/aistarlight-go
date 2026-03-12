package bot

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// duplicateGroup holds a set of transactions suspected as duplicates.
type duplicateGroup struct {
	Amount float64
	Desc   string // representative description
	Txns   []auditTxn
	Reason string // e.g. "same day", "2 days apart"
}

type auditTxn struct {
	RefNumber int32
	Date      time.Time
	Submitter string
	Desc      string
}

func (b *Bot) handleAudit(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Account not linked. Use /link <api_key> first.")
	}

	// Parse period argument: /audit [YYYY-MM]
	args := strings.TrimSpace(c.Message().Payload)
	period := time.Now().UTC().Format("2006-01")
	if args != "" {
		if _, err := time.Parse("2006-01", args); err != nil {
			return c.Send("Invalid period format. Use: /audit YYYY-MM (e.g., /audit 2026-03)")
		}
		period = args
	}

	company, err := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	if err != nil {
		slog.Error("failed to get company for audit", "error", err)
		return c.Send("Failed to load company info.")
	}
	jCfg := jurisdiction.Get(company.Jurisdiction)

	// Date range for the month.
	startDate, _ := time.Parse("2006-01", period)
	endDate := startDate.AddDate(0, 1, -1)

	var pgStart, pgEnd pgtype.Date
	pgStart.Time = startDate
	pgStart.Valid = true
	pgEnd.Time = endDate
	pgEnd.Valid = true

	txns, err := b.q.ListTransactionsWithSubmitter(ctx, sqlc.ListTransactionsWithSubmitterParams{
		CompanyID: tgUser.CompanyID,
		Date:      pgStart,
		Date_2:    pgEnd,
	})
	if err != nil {
		slog.Error("failed to query transactions for audit", "error", err)
		return c.Send("Failed to query transactions.")
	}

	if len(txns) == 0 {
		return c.Send(fmt.Sprintf("No transactions found for %s.", period))
	}

	groups := findDuplicateGroups(txns)
	report := formatAuditReport(groups, jCfg.CurrencySymbol, period)
	return c.Send(report, tele.ModeHTML)
}

// findDuplicateGroups detects suspicious duplicates among transactions.
// Rule 1: exact — same amount + same date + same description (case-insensitive).
// Rule 2: near  — same amount + date within 3 days + description substring match.
func findDuplicateGroups(txns []sqlc.ListTransactionsWithSubmitterRow) []duplicateGroup {
	// Group by amount (rounded to 2 decimal places).
	type amountKey int64 // amount * 100 as integer
	byAmount := map[amountKey][]sqlc.ListTransactionsWithSubmitterRow{}
	for _, t := range txns {
		f, err := t.Amount.Float64Value()
		if err != nil || !f.Valid {
			continue
		}
		key := amountKey(math.Round(f.Float64 * 100))
		byAmount[key] = append(byAmount[key], t)
	}

	var groups []duplicateGroup
	for _, bucket := range byAmount {
		if len(bucket) < 2 {
			continue
		}
		// Within this bucket, find pairs/clusters that are suspicious.
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
			// Mark used.
			for _, idx := range cluster {
				used[idx] = true
			}

			// Build group.
			g := duplicateGroup{}
			f, _ := bucket[cluster[0]].Amount.Float64Value()
			g.Amount = f.Float64
			g.Desc = descOrFallback(bucket[cluster[0]].Description)

			var minDate, maxDate time.Time
			for _, idx := range cluster {
				t := bucket[idx]
				at := auditTxn{
					Submitter: t.SubmittedByName,
					Desc:      descOrFallback(t.Description),
				}
				if t.RefNumber != nil {
					at.RefNumber = *t.RefNumber
				}
				if t.Date.Valid {
					at.Date = t.Date.Time
					if minDate.IsZero() || t.Date.Time.Before(minDate) {
						minDate = t.Date.Time
					}
					if maxDate.IsZero() || t.Date.Time.After(maxDate) {
						maxDate = t.Date.Time
					}
				}
				g.Txns = append(g.Txns, at)
			}

			days := int(maxDate.Sub(minDate).Hours() / 24)
			switch {
			case days == 0:
				g.Reason = "same day — likely duplicate"
			case days == 1:
				g.Reason = "1 day apart"
			default:
				g.Reason = fmt.Sprintf("%d days apart", days)
			}

			groups = append(groups, g)
		}
	}
	return groups
}

// isSuspiciousPair checks whether two transactions with the same amount look like duplicates.
func isSuspiciousPair(a, b sqlc.ListTransactionsWithSubmitterRow) bool {
	// Dates must be within 3 days.
	if !a.Date.Valid || !b.Date.Valid {
		return false
	}
	daysDiff := int(math.Abs(a.Date.Time.Sub(b.Date.Time).Hours() / 24))
	if daysDiff > 3 {
		return false
	}

	descA := strings.ToLower(strings.TrimSpace(descOrFallback(a.Description)))
	descB := strings.ToLower(strings.TrimSpace(descOrFallback(b.Description)))

	// Exact match.
	if descA == descB {
		return true
	}
	// Substring containment (one description contains the other).
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

// formatAuditReport builds the Telegram message for audit results.
func formatAuditReport(groups []duplicateGroup, currencySymbol, period string) string {
	// Friendly month label: "2026-03" → "Mar 2026"
	t, _ := time.Parse("2006-01", period)
	label := t.Format("Jan 2006")

	if len(groups) == 0 {
		return fmt.Sprintf("No suspicious duplicates found for %s.", label)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>Expense Audit — %s</b>\n", label))
	sb.WriteString(fmt.Sprintf("%d suspicious group(s) found\n", len(groups)))

	for i, g := range groups {
		sb.WriteString(fmt.Sprintf("\n<b>#%d — %s%.2f</b> (%s)\n", i+1, currencySymbol, g.Amount, escapeHTML(g.Desc)))
		for _, tx := range g.Txns {
			ref := ""
			if tx.RefNumber > 0 {
				ref = fmt.Sprintf("#TXN-%d", tx.RefNumber)
			}
			sb.WriteString(fmt.Sprintf("  %s: %s | %s\n", ref, tx.Date.Format("2006-01-02"), escapeHTML(tx.Submitter)))
		}
		sb.WriteString(fmt.Sprintf("  ⚠ %s\n", g.Reason))

		// Telegram message limit safety: truncate at ~3800 chars to leave room.
		if sb.Len() > 3800 {
			sb.WriteString("\n... (truncated, too many groups)")
			break
		}
	}

	return sb.String()
}

// escapeHTML escapes <, >, & for Telegram HTML mode.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
