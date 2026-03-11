package bot

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

func (b *Bot) handleExport(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Account not linked. Use /link <api_key> first.")
	}

	// Parse period argument: /export [YYYY-MM]
	args := strings.TrimSpace(c.Message().Payload)
	period := time.Now().UTC().Format("2006-01")
	if args != "" {
		// Validate format
		if _, err := time.Parse("2006-01", args); err != nil {
			return c.Send("Invalid period format. Use: /export YYYY-MM (e.g., /export 2026-03)")
		}
		period = args
	}

	// Look up company for jurisdiction
	company, err := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	if err != nil {
		slog.Error("failed to get company for export", "error", err)
		return c.Send("Failed to load company info.")
	}
	jCfg := jurisdiction.Get(company.Jurisdiction)

	processing, err := b.B.Send(c.Chat(), fmt.Sprintf("Generating bookkeeping export for %s...", period))
	if err != nil {
		return err
	}

	// Parse date range from period
	startDate, _ := time.Parse("2006-01", period)
	endDate := startDate.AddDate(0, 1, -1) // last day of month

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
		slog.Error("failed to query transactions for export", "error", err)
		_, _ = b.B.Edit(processing, "Failed to query transactions.")
		return nil
	}

	if len(txns) == 0 {
		_, _ = b.B.Edit(processing, fmt.Sprintf("No transactions found for %s.", period))
		return nil
	}

	// Convert to TransactionResponse
	txnResponses := make([]service.TransactionResponse, len(txns))
	for i, t := range txns {
		txnResponses[i] = sqlcTxnWithSubmitterToResponse(t)
	}

	tinNumber := ""
	if company.TinNumber != nil {
		tinNumber = *company.TinNumber
	}
	companyInfo := service.CompanyInfo{
		CompanyName: company.CompanyName,
		TINNumber:   tinNumber,
	}

	// Generate Excel
	var buf bytes.Buffer
	if err := service.GenerateBookkeepingExcel(&buf, txnResponses, period, companyInfo, jCfg.CurrencySymbol); err != nil {
		slog.Error("failed to generate bookkeeping excel", "error", err)
		_, _ = b.B.Edit(processing, "Failed to generate Excel file.")
		return nil
	}

	_, _ = b.B.Edit(processing, fmt.Sprintf("Sending %d transactions for %s...", len(txns), period))

	filename := fmt.Sprintf("bookkeeping_%s_%s.xlsx", company.CompanyName, period)
	doc := &tele.Document{
		File:     tele.FromReader(&buf),
		FileName: filename,
		Caption:  fmt.Sprintf("Bookkeeping export: %s (%d transactions)", period, len(txns)),
	}

	return c.Send(doc)
}

// sqlcTxnWithSubmitterToResponse converts a ListTransactionsWithSubmitterRow to service.TransactionResponse.
func sqlcTxnWithSubmitterToResponse(t sqlc.ListTransactionsWithSubmitterRow) service.TransactionResponse {
	resp := service.TransactionResponse{
		ID:                   t.ID.String(),
		SourceType:           t.SourceType,
		SourceFileID:         t.SourceFileID,
		RowIndex:             int(t.RowIndex),
		Description:          t.Description,
		VATType:              t.VatType,
		Category:             t.Category,
		TIN:                  t.Tin,
		ClassificationSource: t.ClassificationSource,
		MatchStatus:          t.MatchStatus,
		ATCCode:              t.AtcCode,
		SubmittedByName:      &t.SubmittedByName,
	}
	if t.Date.Valid {
		d := t.Date.Time.Format("2006-01-02")
		resp.Date = &d
	}
	if f, err := t.Amount.Float64Value(); err == nil {
		resp.Amount = f.Float64
	}
	if f, err := t.VatAmount.Float64Value(); err == nil {
		resp.VATAmount = f.Float64
	}
	if f, err := t.Confidence.Float64Value(); err == nil {
		resp.Confidence = f.Float64
	}
	return resp
}
