package bot

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// Pre-compiled regexps for natural language correction parsing.
var (
	reCorrAmount    = regexp.MustCompile(`(?:金额|总额|amount).*?(\d[\d,]*\.?\d*)`)
	reCorrAmountAlt = regexp.MustCompile(`(?:改|change|update).*?(?:金额|总额|amount).*?(\d[\d,]*\.?\d*)`)
	reCorrVendor    = regexp.MustCompile(`(?:商家|描述|vendor|description).*?(?:改成|改为|变成|change to|to)\s*(.+)`)
	reCorrCategory  = regexp.MustCompile(`(?:类别|分类|category).*?(?:改成|改为|变成|change to|to)\s*(.+)`)
)

// handleTransactionCorrection processes a reply-to correction.
// The user replies to a "Receipt Recorded" or "Exchange Recorded" message
// with corrections like "amount: 2000" or "金额改成3000".
func (b *Bot) handleTransactionCorrection(c tele.Context, data *ReplyTxnData, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Account not linked. Use /link <api_key> first.")
	}

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	// Parse corrections from user text.
	corrections := parseCorrectionText(text)
	if len(corrections) == 0 {
		return c.Send("Could not parse corrections. Use format:\namount 2000\nvendor ABC Store\ndate 2025-01-15\ncategory services\nvat 120")
	}

	// Apply corrections to all transactions in this reply group.
	var updated []string
	for i, txnID := range data.TxnIDs {
		params := sqlc.UpdateTransactionFieldsParams{
			ID:        txnID,
			CompanyID: tgUser.CompanyID,
		}

		var changes []string

		if v, ok := corrections["amount"]; ok {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil {
				params.SetAmount = true
				var num pgtype.Numeric
				_ = num.Scan(fmt.Sprintf("%.2f", f))
				params.NewAmount = num
				changes = append(changes, fmt.Sprintf("amount -> %s%s", jCfg.CurrencySymbol, addCommas(fmt.Sprintf("%.2f", f))))
			}
		}

		if v, ok := corrections["description"]; ok {
			params.SetDescription = true
			params.NewDescription = v
			changes = append(changes, fmt.Sprintf("description -> %s", v))
		}

		if v, ok := corrections["date"]; ok {
			if parsed, err := time.Parse("2006-01-02", v); err == nil {
				params.SetDate = true
				params.NewDate = pgtype.Date{Time: parsed, Valid: true}
				changes = append(changes, fmt.Sprintf("date -> %s", v))
			}
		}

		if v, ok := corrections["category"]; ok {
			params.SetCategory = true
			params.NewCategory = v
			changes = append(changes, fmt.Sprintf("category -> %s", v))
		}

		if v, ok := corrections["vat"]; ok {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil {
				params.SetVatAmount = true
				var num pgtype.Numeric
				_ = num.Scan(fmt.Sprintf("%.2f", f))
				params.NewVatAmount = num
				changes = append(changes, fmt.Sprintf("vat -> %s%s", jCfg.CurrencySymbol, addCommas(fmt.Sprintf("%.2f", f))))
			}
		}

		if len(changes) == 0 {
			continue
		}

		_, err := b.q.UpdateTransactionFields(ctx, params)
		if err != nil {
			slog.Error("correction update failed", "txn_id", txnID, "error", err)
			continue
		}

		refLabel := ""
		if i < len(data.RefNumbers) && data.RefNumbers[i] > 0 {
			refLabel = fmt.Sprintf("#TXN-%d", data.RefNumbers[i])
		}
		updated = append(updated, fmt.Sprintf("Updated %s: %s", refLabel, strings.Join(changes, ", ")))
	}

	if len(updated) == 0 {
		return c.Send("No transactions were updated. Please check your input.")
	}

	return c.Send(strings.Join(updated, "\n"))
}

// parseCorrectionText parses correction input from user text.
// Supports key:value format and natural language patterns.
// Returns a map of field name -> value.
func parseCorrectionText(text string) map[string]string {
	corrections := make(map[string]string)

	// Known field aliases mapped to canonical names.
	fieldAliases := map[string]string{
		"amount": "amount", "金额": "amount", "总额": "amount",
		"vendor": "description", "description": "description", "商家": "description", "描述": "description", "说明": "description",
		"date": "date", "日期": "date",
		"category": "category", "类别": "category", "分类": "category",
		"vat": "vat", "税额": "vat", "增值税": "vat",
		"tin": "tin",
		"receipt_no": "receipt_no", "ref": "receipt_no", "receipt": "receipt_no",
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var key, value string
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			key = strings.TrimSpace(strings.ToLower(parts[0]))
			value = strings.TrimSpace(parts[1])
		} else if parts := strings.SplitN(line, "：", 2); len(parts) == 2 { // Chinese colon
			key = strings.TrimSpace(strings.ToLower(parts[0]))
			value = strings.TrimSpace(parts[1])
		} else if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			// Space-separated: only accept if first word is a known field.
			candidate := strings.TrimSpace(strings.ToLower(parts[0]))
			if _, ok := fieldAliases[candidate]; ok {
				key = candidate
				value = strings.TrimSpace(parts[1])
			}
		}

		if value == "" {
			continue
		}

		if canonical, ok := fieldAliases[key]; ok {
			corrections[canonical] = value
		}
	}

	// If key:value parsing found results, return them.
	if len(corrections) > 0 {
		return corrections
	}

	// Try natural language patterns.
	lower := strings.ToLower(text)

	// Amount patterns: "金额改成3000", "amount should be 2000", "改金额2000"
	for _, re := range []*regexp.Regexp{reCorrAmount, reCorrAmountAlt} {
		if m := re.FindStringSubmatch(lower); len(m) > 1 {
			corrections["amount"] = strings.ReplaceAll(m[1], ",", "")
			break
		}
	}

	// Vendor/description patterns: "商家改成ABC", "vendor to ABC"
	if m := reCorrVendor.FindStringSubmatch(lower); len(m) > 1 {
		corrections["description"] = strings.TrimSpace(m[1])
	}

	// Category patterns: "类别改成services", "category to goods"
	if m := reCorrCategory.FindStringSubmatch(lower); len(m) > 1 {
		corrections["category"] = strings.TrimSpace(m[1])
	}

	return corrections
}
