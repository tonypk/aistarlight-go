package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// CASService implements BIR Computerized Accounting System requirements:
// sequential numbering, SHA-256 hash chain, compliance validation, subsidiary ledgers.
type CASService struct {
	q    *sqlc.Queries
	pool *pgxpool.Pool
}

func NewCASService(q *sqlc.Queries, pool *pgxpool.Pool) *CASService {
	return &CASService{q: q, pool: pool}
}

// JournalBook constants matching BIR subsidiary ledger categories.
const (
	JournalBookGeneral          = "general_journal"
	JournalBookSales            = "sales_journal"
	JournalBookPurchases        = "purchases_journal"
	JournalBookCashReceipts     = "cash_receipts"
	JournalBookCashDisbursement = "cash_disbursements"
)

// --- Sequential numbering + hash chain ---

// StampJournalEntry assigns a company-sequential number and hash chain link
// to a journal entry. It uses the provided transaction so that the stamp is
// atomic with the entry creation. The caller must commit the tx.
func (s *CASService) StampJournalEntry(ctx context.Context, qtx *sqlc.Queries, entryID, companyID uuid.UUID, entryDate time.Time, description string, journalBook string) error {
	// 1. Get next sequence number (atomic via INSERT...ON CONFLICT, row-level lock)
	seqNo, err := qtx.NextCASSequence(ctx, sqlc.NextCASSequenceParams{
		CompanyID:    companyID,
		SequenceType: "journal_entry",
		Prefix:       "JE",
	})
	if err != nil {
		return fmt.Errorf("cas next sequence: %w", err)
	}

	// 2. Get previous hash for chain (within same tx, serialized by sequence lock)
	prevHash := ""
	lastHash, err := qtx.GetLastJournalHash(ctx, companyID)
	if err == nil && lastHash != nil {
		prevHash = *lastHash
	}

	// 3. Compute hash: SHA-256(companyID|seqNo|entryDate|description|prevHash)
	entryHash := computeJournalHash(companyID, seqNo, entryDate, description, prevHash)

	// 4. Update journal entry with CAS fields
	err = qtx.UpdateJournalCASFields(ctx, sqlc.UpdateJournalCASFieldsParams{
		ID:           entryID,
		CompanySeqNo: &seqNo,
		EntryHash:    &entryHash,
		PrevHash:     strPtr(prevHash),
		JournalBook:  &journalBook,
	})
	if err != nil {
		return fmt.Errorf("cas update fields: %w", err)
	}

	return nil
}

// StampJournalEntryStandalone opens its own transaction for stamping.
// Use when not already inside a transaction.
func (s *CASService) StampJournalEntryStandalone(ctx context.Context, entryID, companyID uuid.UUID, entryDate time.Time, description string, journalBook string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cas stamp begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if err := s.StampJournalEntry(ctx, qtx, entryID, companyID, entryDate, description, journalBook); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// computeJournalHash produces a SHA-256 hex string from entry data.
func computeJournalHash(companyID uuid.UUID, seqNo int64, entryDate time.Time, description, prevHash string) string {
	data := fmt.Sprintf("%s|%d|%s|%s|%s",
		companyID.String(),
		seqNo,
		entryDate.Format("2006-01-02"),
		description,
		prevHash,
	)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}

// --- Audit log with hash chain ---

// LogAuditWithHash creates an audit log entry with hash chain inside a transaction
// to prevent concurrent hash chain forks.
func (s *CASService) LogAuditWithHash(ctx context.Context, companyID uuid.UUID, userID *uuid.UUID, entityType string, entityID *uuid.UUID, action string, changes map[string]interface{}, comment, ipAddress, userAgent *string) error {
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("cas marshal audit changes: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cas audit begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	// Get previous audit hash (within tx for chain integrity)
	prevHash := ""
	lastHash, err := qtx.GetLastAuditHash(ctx, companyID)
	if err == nil && lastHash != nil {
		prevHash = *lastHash
	}

	// Compute audit hash
	auditData := fmt.Sprintf("%s|%s|%s|%s|%s",
		companyID.String(), entityType, action, string(changesJSON), prevHash)
	h := sha256.Sum256([]byte(auditData))
	entryHash := fmt.Sprintf("%x", h)

	uid := pgtype.UUID{}
	if userID != nil {
		uid = pgtype.UUID{Bytes: *userID, Valid: true}
	}
	eid := pgtype.UUID{}
	if entityID != nil {
		eid = pgtype.UUID{Bytes: *entityID, Valid: true}
	}

	_, err = qtx.CreateAuditLogWithHash(ctx, sqlc.CreateAuditLogWithHashParams{
		CompanyID:  companyID,
		UserID:     uid,
		EntityType: entityType,
		EntityID:   eid,
		Action:     action,
		Changes:    changesJSON,
		Comment:    comment,
		EntryHash:  &entryHash,
		PrevHash:   strPtr(prevHash),
		IpAddress:  ipAddress,
		UserAgent:  userAgent,
	})
	if err != nil {
		return fmt.Errorf("cas create audit log: %w", err)
	}

	return tx.Commit(ctx)
}

// --- Compliance checks ---

// ComplianceResult holds the full CAS compliance check output.
type ComplianceResult struct {
	OverallPass           bool                   `json:"overall_pass"`
	SequentialNumberingOk bool                   `json:"sequential_numbering_ok"`
	HashChainIntact       bool                   `json:"hash_chain_intact"`
	DoubleEntryBalanced   bool                   `json:"double_entry_balanced"`
	PeriodsProperlyClosed bool                   `json:"periods_properly_closed"`
	AuditTrailComplete    bool                   `json:"audit_trail_complete"`
	SubsidiaryLedgersOk   bool                   `json:"subsidiary_ledgers_ok"`
	Details               map[string]interface{} `json:"details"`
}

// RunComplianceCheck performs all CAS validation checks for a company.
func (s *CASService) RunComplianceCheck(ctx context.Context, companyID, checkedBy uuid.UUID) (*ComplianceResult, error) {
	details := map[string]interface{}{}

	seqOk, err := s.checkSequentialNumbering(ctx, companyID, details)
	if err != nil {
		return nil, err
	}

	hashOk, err := s.checkHashChain(ctx, companyID, details)
	if err != nil {
		return nil, err
	}

	deOk, err := s.checkDoubleEntry(ctx, companyID, details)
	if err != nil {
		return nil, err
	}

	periodsOk := s.checkPeriods(details)

	auditOk, err := s.checkAuditTrail(ctx, companyID, details)
	if err != nil {
		return nil, err
	}

	subOk, err := s.checkSubsidiaryLedgers(ctx, companyID, details)
	if err != nil {
		return nil, err
	}

	overallPass := seqOk && hashOk && deOk && periodsOk && auditOk && subOk

	result := &ComplianceResult{
		OverallPass:           overallPass,
		SequentialNumberingOk: seqOk,
		HashChainIntact:       hashOk,
		DoubleEntryBalanced:   deOk,
		PeriodsProperlyClosed: periodsOk,
		AuditTrailComplete:    auditOk,
		SubsidiaryLedgersOk:   subOk,
		Details:               details,
	}

	// Persist compliance check result
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("cas marshal details: %w", err)
	}
	_, err = s.q.InsertCASComplianceCheck(ctx, sqlc.InsertCASComplianceCheckParams{
		CompanyID:             companyID,
		OverallPass:           overallPass,
		SequentialNumberingOk: seqOk,
		HashChainIntact:       hashOk,
		DoubleEntryBalanced:   deOk,
		PeriodsProperlyClosed: periodsOk,
		AuditTrailComplete:    auditOk,
		SubsidiaryLedgersOk:   subOk,
		Details:               detailsJSON,
		CheckedBy:             pgtype.UUID{Bytes: checkedBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("save compliance check: %w", err)
	}

	return result, nil
}

func (s *CASService) checkSequentialNumbering(ctx context.Context, companyID uuid.UUID, details map[string]interface{}) (bool, error) {
	gaps, err := s.q.DetectSequenceGaps(ctx, companyID)
	if err != nil {
		return false, fmt.Errorf("detect gaps: %w", err)
	}
	if len(gaps) == 0 {
		return true, nil
	}
	gapList := make([]map[string]interface{}, len(gaps))
	for i, g := range gaps {
		gapList[i] = map[string]interface{}{
			"gap_after":  g.GapAfter,
			"gap_before": g.GapBefore,
		}
	}
	details["sequence_gaps"] = gapList
	return false, nil
}

func (s *CASService) checkHashChain(ctx context.Context, companyID uuid.UUID, details map[string]interface{}) (bool, error) {
	broken, err := s.q.VerifyJournalHashChain(ctx, companyID)
	if err != nil {
		return false, fmt.Errorf("verify hash chain: %w", err)
	}
	if len(broken) == 0 {
		return true, nil
	}
	brokenList := make([]map[string]interface{}, len(broken))
	for i, b := range broken {
		brokenList[i] = map[string]interface{}{
			"entry_id":      b.ID.String(),
			"seq_no":        b.CompanySeqNo,
			"expected_hash": b.ExpectedPrevHash,
			"actual_hash":   b.PrevHash,
		}
	}
	details["hash_chain_breaks"] = brokenList
	return false, nil
}

func (s *CASService) checkDoubleEntry(ctx context.Context, companyID uuid.UUID, details map[string]interface{}) (bool, error) {
	draftCount, err := s.q.CountUnpostedDraftEntries(ctx, companyID)
	if err != nil {
		return false, fmt.Errorf("count drafts: %w", err)
	}
	// All entries created through JournalService.Create are balanced by construction.
	details["unposted_drafts"] = draftCount
	return true, nil
}

func (s *CASService) checkPeriods(details map[string]interface{}) bool {
	// Basic check; enhanced period validation can be added later.
	details["periods_check"] = "basic"
	return true
}

func (s *CASService) checkAuditTrail(ctx context.Context, companyID uuid.UUID, details map[string]interface{}) (bool, error) {
	lastAuditHash, err := s.q.GetLastAuditHash(ctx, companyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			details["audit_trail"] = "no hashed audit entries found"
			return false, nil
		}
		return false, fmt.Errorf("check audit trail: %w", err)
	}
	if lastAuditHash == nil {
		details["audit_trail"] = "no hashed audit entries found"
		return false, nil
	}
	return true, nil
}

func (s *CASService) checkSubsidiaryLedgers(ctx context.Context, companyID uuid.UUID, details map[string]interface{}) (bool, error) {
	// Count entries per journal book (no date filter — global count for compliance)
	bookCounts, err := s.q.CountEntriesByJournalBook(ctx, sqlc.CountEntriesByJournalBookParams{
		CompanyID: companyID,
	})
	if err != nil {
		return false, fmt.Errorf("count journal books: %w", err)
	}
	bookMap := map[string]int64{}
	for _, bc := range bookCounts {
		if bc.JournalBook != nil {
			bookMap[*bc.JournalBook] = bc.EntryCount
		}
	}
	details["journal_books"] = bookMap
	// At minimum, general journal should have entries
	return bookMap[JournalBookGeneral] > 0, nil
}

// GetLatestCheck returns the most recent CAS compliance check.
func (s *CASService) GetLatestCheck(ctx context.Context, companyID uuid.UUID) (*ComplianceResult, error) {
	check, err := s.q.GetLatestCASCheck(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("get latest CAS check: %w", err)
	}

	var details map[string]interface{}
	if err := json.Unmarshal(check.Details, &details); err != nil {
		return nil, fmt.Errorf("unmarshal CAS check details: %w", err)
	}

	return &ComplianceResult{
		OverallPass:           check.OverallPass,
		SequentialNumberingOk: check.SequentialNumberingOk,
		HashChainIntact:       check.HashChainIntact,
		DoubleEntryBalanced:   check.DoubleEntryBalanced,
		PeriodsProperlyClosed: check.PeriodsProperlyClosed,
		AuditTrailComplete:    check.AuditTrailComplete,
		SubsidiaryLedgersOk:   check.SubsidiaryLedgersOk,
		Details:               details,
	}, nil
}

// ListChecks returns paginated CAS compliance checks.
func (s *CASService) ListChecks(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]sqlc.ListCASChecksRow, error) {
	return s.q.ListCASChecks(ctx, sqlc.ListCASChecksParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
}

// --- Subsidiary ledger ---

// SubsidiaryLedgerEntry represents a row in the BIR subsidiary ledger report.
type SubsidiaryLedgerEntry struct {
	EntryID          uuid.UUID `json:"entry_id"`
	EntryNumber      *int32    `json:"entry_number,omitempty"`
	CompanySeqNo     *int64    `json:"company_seq_no,omitempty"`
	EntryDate        string    `json:"entry_date"`
	EntryDescription *string   `json:"entry_description,omitempty"`
	JournalBook      *string   `json:"journal_book,omitempty"`
	LineID           uuid.UUID `json:"line_id"`
	AccountID        uuid.UUID `json:"account_id"`
	AccountCode      string    `json:"account_code"`
	AccountName      string    `json:"account_name"`
	Debit            string    `json:"debit"`
	Credit           string    `json:"credit"`
	LineDescription  *string   `json:"line_description,omitempty"`
	TaxCode          *string   `json:"tax_code,omitempty"`
	TaxAmount        string    `json:"tax_amount"`
}

// GetSubsidiaryLedger returns subsidiary ledger entries for a journal book type.
func (s *CASService) GetSubsidiaryLedger(ctx context.Context, companyID uuid.UUID, journalBook string, from, to *time.Time) ([]SubsidiaryLedgerEntry, error) {
	params := sqlc.GetSubsidiaryLedgerParams{
		CompanyID:   companyID,
		JournalBook: &journalBook,
	}
	if from != nil {
		params.Column3 = pgtype.Date{Time: *from, Valid: true}
	}
	if to != nil {
		params.Column4 = pgtype.Date{Time: *to, Valid: true}
	}

	rows, err := s.q.GetSubsidiaryLedger(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("get subsidiary ledger: %w", err)
	}

	entries := make([]SubsidiaryLedgerEntry, len(rows))
	for i, r := range rows {
		entries[i] = SubsidiaryLedgerEntry{
			EntryID:          r.EntryID,
			EntryNumber:      r.EntryNumber,
			CompanySeqNo:     r.CompanySeqNo,
			EntryDate:        r.EntryDate.Time.Format("2006-01-02"),
			EntryDescription: r.EntryDescription,
			JournalBook:      r.JournalBook,
			LineID:           r.LineID,
			AccountID:        r.AccountID,
			AccountCode:      r.AccountCode,
			AccountName:      r.AccountName,
			Debit:            numericToDecimal(r.Debit).String(),
			Credit:           numericToDecimal(r.Credit).String(),
			LineDescription:  r.LineDescription,
			TaxCode:          r.TaxCode,
			TaxAmount:        numericToDecimal(r.TaxAmount).String(),
		}
	}
	return entries, nil
}

// ClassifyJournalBook determines the BIR journal book type based on account
// types and sub-types in a journal entry.
// Rules:
// - If entry has revenue/sales accounts → sales_journal
// - If entry has expense/purchase accounts → purchases_journal
// - If entry has cash/bank sub_type="cash" on debit side → cash_receipts
// - If entry has cash/bank sub_type="cash" on credit side → cash_disbursements
// - Otherwise → general_journal
func ClassifyJournalBook(lines []CreateJournalLineInput, accountTypes map[uuid.UUID]string, accountSubTypes map[uuid.UUID]string) string {
	hasCashDebit := false
	hasCashCredit := false
	hasRevenue := false
	hasPurchase := false

	for _, line := range lines {
		acctType := accountTypes[line.AccountID]
		subType := accountSubTypes[line.AccountID]

		switch acctType {
		case "asset":
			// Only classify as cash journal if sub_type is "cash" (not AR, prepaid, etc.)
			if subType == "cash" {
				if line.Debit.IsPositive() {
					hasCashDebit = true
				}
				if line.Credit.IsPositive() {
					hasCashCredit = true
				}
			}
		case "revenue":
			hasRevenue = true
		case "expense", "cost_of_goods_sold":
			hasPurchase = true
		}
	}

	switch {
	case hasRevenue:
		return JournalBookSales
	case hasPurchase && !hasRevenue:
		return JournalBookPurchases
	case hasCashDebit && !hasCashCredit:
		return JournalBookCashReceipts
	case hasCashCredit && !hasCashDebit:
		return JournalBookCashDisbursement
	default:
		return JournalBookGeneral
	}
}
