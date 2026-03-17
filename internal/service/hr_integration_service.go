package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	integration "github.com/tonypk/aistarlight-go/internal/domain/integration"
	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// HRIntegrationService processes inbound events from AIGoNHR.
type HRIntegrationService struct {
	q          *sqlc.Queries
	glMapping  *GLMappingService
	journalSvc *JournalService
	taxBridge  *PayrollTaxBridge
	logger     *slog.Logger
}

// NewHRIntegrationService creates a new integration processor.
func NewHRIntegrationService(
	q *sqlc.Queries,
	glMapping *GLMappingService,
	journalSvc *JournalService,
	taxBridge *PayrollTaxBridge,
	logger *slog.Logger,
) *HRIntegrationService {
	return &HRIntegrationService{
		q:          q,
		glMapping:  glMapping,
		journalSvc: journalSvc,
		taxBridge:  taxBridge,
		logger:     logger,
	}
}

// ProcessEvent dispatches an inbox event to the appropriate handler.
func (s *HRIntegrationService) ProcessEvent(ctx context.Context, evt sqlc.IntegrationEventInbox) error {
	switch evt.EventType {
	case integration.EventPayrollRunCompleted:
		return s.handlePayrollRunCompleted(ctx, evt)
	case integration.EventEmployeeUpserted:
		return s.handleEmployeeUpserted(ctx, evt)
	case integration.EventPayrollRunReversed:
		return s.handlePayrollRunReversed(ctx, evt)
	case integration.EventEmployeeTerminated:
		return s.handleEmployeeTerminated(ctx, evt)
	default:
		s.logger.Warn("unknown event type", "event_type", evt.EventType, "event_id", evt.EventID)
		return nil
	}
}

// handlePayrollRunCompleted creates journal entries from a payroll run event.
func (s *HRIntegrationService) handlePayrollRunCompleted(ctx context.Context, evt sqlc.IntegrationEventInbox) error {
	// Parse the webhook payload wrapper, then extract the nested event data
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(evt.Payload, &wrapper); err != nil {
		return fmt.Errorf("unmarshal webhook wrapper: %w", err)
	}

	var payrollEvt integration.PayrollRunCompletedEvent
	data := wrapper.Data
	if data == nil {
		data = evt.Payload // fallback: payload IS the event
	}
	if err := json.Unmarshal(data, &payrollEvt); err != nil {
		return fmt.Errorf("unmarshal payroll event: %w", err)
	}

	// Load GL mappings for this company + jurisdiction
	payDate, err := time.Parse("2006-01-02", payrollEvt.PayDate)
	if err != nil {
		return fmt.Errorf("parse pay_date: %w", err)
	}

	mappingIdx, err := s.glMapping.BuildIndex(ctx, evt.CompanyID, payrollEvt.Jurisdiction, payDate)
	if err != nil {
		return fmt.Errorf("build GL mapping index: %w", err)
	}
	if len(mappingIdx) == 0 {
		return fmt.Errorf("no GL mappings configured for company %s jurisdiction %s", evt.CompanyID, payrollEvt.Jurisdiction)
	}

	// Build journal lines
	var lines []CreateJournalLineInput

	// Process each employee's earnings as debits
	for _, emp := range payrollEvt.EmployeeLines {
		for _, earning := range emp.Earnings {
			mapping, ok := mappingIdx["earning:"+earning.Code]
			if !ok {
				s.logger.Warn("no GL mapping for earning", "code", earning.Code)
				continue
			}
			amount, _ := decimal.NewFromString(earning.Amount)
			if amount.IsZero() {
				continue
			}
			lines = append(lines, CreateJournalLineInput{
				AccountID:   mapping.TargetAccountID,
				Description: strPtr(fmt.Sprintf("%s - %s", earning.Label, emp.FullName)),
				Debit:       amount,
				Credit:      decimal.Zero,
			})
		}
	}

	// Statutory payables as credits (employee portions) + debits (employer portions)
	for _, stat := range payrollEvt.StatutoryPayables {
		// Employee contribution → credit to payable
		eeMapping, ok := mappingIdx["deduction:"+stat.Code+"_employee"]
		if ok {
			eeAmt, _ := decimal.NewFromString(stat.EmployeeAmount)
			if !eeAmt.IsZero() {
				lines = append(lines, CreateJournalLineInput{
					AccountID:   eeMapping.TargetAccountID,
					Description: strPtr(stat.Label + " Employee"),
					Debit:       decimal.Zero,
					Credit:      eeAmt,
				})
			}
		}

		// Employer contribution → debit expense + credit payable
		erExpMapping, ok := mappingIdx["contribution:"+stat.Code+"_employer"]
		if ok {
			erAmt, _ := decimal.NewFromString(stat.EmployerAmount)
			if !erAmt.IsZero() {
				lines = append(lines, CreateJournalLineInput{
					AccountID:   erExpMapping.TargetAccountID,
					Description: strPtr(stat.Label + " Employer Expense"),
					Debit:       erAmt,
					Credit:      decimal.Zero,
				})
				// Credit to the same payable account as employee
				if eeMapping, ok := mappingIdx["deduction:"+stat.Code+"_employee"]; ok {
					lines = append(lines, CreateJournalLineInput{
						AccountID:   eeMapping.TargetAccountID,
						Description: strPtr(stat.Label + " Employer Payable"),
						Debit:       decimal.Zero,
						Credit:      erAmt,
					})
				}
			}
		}
	}

	// Withholding tax → credit
	totalWHT := decimal.Zero
	for _, wh := range payrollEvt.WithholdingLines {
		amt, _ := decimal.NewFromString(wh.TaxAmount)
		totalWHT = totalWHT.Add(amt)
	}
	if !totalWHT.IsZero() {
		if whtMapping, ok := mappingIdx["deduction:withholding_tax"]; ok {
			lines = append(lines, CreateJournalLineInput{
				AccountID:   whtMapping.TargetAccountID,
				Description: strPtr("Withholding Tax Payable"),
				Debit:       decimal.Zero,
				Credit:      totalWHT,
			})
		}
	}

	// Net pay → credit cash/bank
	netPay, _ := decimal.NewFromString(payrollEvt.Totals.NetPay)
	if !netPay.IsZero() {
		if cashMapping, ok := mappingIdx["net_pay:cash"]; ok {
			lines = append(lines, CreateJournalLineInput{
				AccountID:   cashMapping.TargetAccountID,
				Description: strPtr("Net Pay - Payroll Bank"),
				Debit:       decimal.Zero,
				Credit:      netPay,
			})
		}
	}

	if len(lines) < 2 {
		return fmt.Errorf("insufficient GL mapping: only %d journal lines generated", len(lines))
	}

	// Consolidate lines by account (sum amounts per account)
	lines = consolidateLines(lines)

	// Create journal entry
	sourceType := "payroll_run"
	ref := fmt.Sprintf("PR-%d", payrollEvt.PayrollRunID)
	desc := fmt.Sprintf("Payroll: %s (%s to %s)", payrollEvt.CycleName, payrollEvt.PeriodStart, payrollEvt.PeriodEnd)

	// Use a system user UUID for created_by (zero UUID = system)
	systemUser := uuid.Nil

	entry, err := s.journalSvc.Create(ctx, CreateJournalEntryInput{
		CompanyID:   evt.CompanyID,
		EntryDate:   payDate,
		Reference:   &ref,
		Description: &desc,
		SourceType:  &sourceType,
		Memo:        strPtr(fmt.Sprintf("Auto-generated from AIGoNHR payroll run #%d, %d employees", payrollEvt.PayrollRunID, payrollEvt.Totals.HeadCount)),
		CreatedBy:   systemUser,
		Lines:       lines,
	})
	if err != nil {
		return fmt.Errorf("create journal entry: %w", err)
	}

	s.logger.Info("payroll journal created",
		"journal_id", entry.ID,
		"company_id", evt.CompanyID,
		"payroll_run_id", payrollEvt.PayrollRunID,
		"lines", len(lines),
	)

	// Auto-fill tax drafts (1601C + 2316 YTD) — best-effort, don't fail the event
	if s.taxBridge != nil {
		if err := s.taxBridge.ProcessPayrollForTax(ctx, evt.CompanyID, &payrollEvt); err != nil {
			s.logger.Warn("tax bridge processing failed", "error", err, "payroll_run_id", payrollEvt.PayrollRunID)
		}
	}

	return nil
}

// handleEmployeeUpserted syncs an employee as an HR payee.
func (s *HRIntegrationService) handleEmployeeUpserted(ctx context.Context, evt sqlc.IntegrationEventInbox) error {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(evt.Payload, &wrapper); err != nil {
		return fmt.Errorf("unmarshal wrapper: %w", err)
	}

	var empEvt integration.EmployeeUpsertedEvent
	data := wrapper.Data
	if data == nil {
		data = evt.Payload
	}
	if err := json.Unmarshal(data, &empEvt); err != nil {
		return fmt.Errorf("unmarshal employee event: %w", err)
	}

	_, err := s.q.UpsertHRPayee(ctx, sqlc.UpsertHRPayeeParams{
		CompanyID:      evt.CompanyID,
		HrEmployeeID:   empEvt.EmployeeID,
		HrEmployeeNo:   empEvt.EmployeeNo,
		FirstName:      empEvt.FirstName,
		LastName:       empEvt.LastName,
		Email:          nilStr(empEvt.Email),
		Tin:            nilStr(empEvt.TIN),
		Sss:            nilStr(empEvt.SSS),
		Philhealth:     nilStr(empEvt.PhilHealth),
		Pagibig:        nilStr(empEvt.PagIBIG),
		DepartmentName: nilStr(empEvt.DepartmentName),
		PositionTitle:  nilStr(empEvt.PositionTitle),
		Jurisdiction:   empEvt.Jurisdiction,
		Status:         empEvt.Status,
	})
	if err != nil {
		return fmt.Errorf("upsert hr payee: %w", err)
	}

	s.logger.Info("hr payee synced",
		"employee_id", empEvt.EmployeeID,
		"employee_no", empEvt.EmployeeNo,
		"company_id", evt.CompanyID,
	)
	return nil
}

// handlePayrollRunReversed reverses the journal entry created by a payroll run.
func (s *HRIntegrationService) handlePayrollRunReversed(ctx context.Context, evt sqlc.IntegrationEventInbox) error {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(evt.Payload, &wrapper); err != nil {
		return fmt.Errorf("unmarshal wrapper: %w", err)
	}

	var revEvt integration.PayrollRunReversedEvent
	data := wrapper.Data
	if data == nil {
		data = evt.Payload
	}
	if err := json.Unmarshal(data, &revEvt); err != nil {
		return fmt.Errorf("unmarshal reversal event: %w", err)
	}

	// Find the original journal entry by source reference
	sourceType := "payroll_run"
	ref := fmt.Sprintf("PR-%d", revEvt.PayrollRunID)
	original, err := s.q.FindJournalEntryBySourceRef(ctx, sqlc.FindJournalEntryBySourceRefParams{
		CompanyID:  evt.CompanyID,
		SourceType: &sourceType,
		Reference:  &ref,
	})
	if err != nil {
		return fmt.Errorf("find original journal for PR-%d: %w", revEvt.PayrollRunID, err)
	}

	// Use system user for the reversal
	systemUser := uuid.Nil

	// If draft, post it first so we can reverse it
	if original.Status == "draft" {
		if err := s.journalSvc.Post(ctx, original.ID, systemUser); err != nil {
			return fmt.Errorf("post draft journal before reversal: %w", err)
		}
	}

	reversal, err := s.journalSvc.Reverse(ctx, original.ID, systemUser)
	if err != nil {
		return fmt.Errorf("reverse journal entry: %w", err)
	}

	s.logger.Info("payroll journal reversed",
		"original_journal_id", original.ID,
		"reversal_journal_id", reversal.ID,
		"payroll_run_id", revEvt.PayrollRunID,
		"reason", revEvt.ReversalReason,
	)

	return nil
}

// handleEmployeeTerminated marks an HR payee as terminated.
func (s *HRIntegrationService) handleEmployeeTerminated(ctx context.Context, evt sqlc.IntegrationEventInbox) error {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(evt.Payload, &wrapper); err != nil {
		return fmt.Errorf("unmarshal wrapper: %w", err)
	}

	var termEvt integration.EmployeeTerminatedEvent
	data := wrapper.Data
	if data == nil {
		data = evt.Payload
	}
	if err := json.Unmarshal(data, &termEvt); err != nil {
		return fmt.Errorf("unmarshal termination event: %w", err)
	}

	if err := s.q.TerminateHRPayee(ctx, sqlc.TerminateHRPayeeParams{
		CompanyID:    evt.CompanyID,
		HrEmployeeID: termEvt.EmployeeID,
	}); err != nil {
		return fmt.Errorf("terminate hr payee: %w", err)
	}

	s.logger.Info("hr payee terminated",
		"employee_id", termEvt.EmployeeID,
		"employee_no", termEvt.EmployeeNo,
		"company_id", evt.CompanyID,
		"reason", termEvt.Reason,
	)
	return nil
}

// consolidateLines merges lines with the same account ID.
func consolidateLines(lines []CreateJournalLineInput) []CreateJournalLineInput {
	type key struct {
		accountID uuid.UUID
		isDebit   bool
	}
	merged := map[key]*CreateJournalLineInput{}
	var order []key

	for _, l := range lines {
		isDebit := l.Debit.GreaterThan(decimal.Zero)
		k := key{accountID: l.AccountID, isDebit: isDebit}
		if existing, ok := merged[k]; ok {
			existing.Debit = existing.Debit.Add(l.Debit)
			existing.Credit = existing.Credit.Add(l.Credit)
		} else {
			copy := l
			merged[k] = &copy
			order = append(order, k)
		}
	}

	result := make([]CreateJournalLineInput, 0, len(order))
	for _, k := range order {
		m := merged[k]
		if m.Debit.IsZero() && m.Credit.IsZero() {
			continue
		}
		result = append(result, *m)
	}
	return result
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
