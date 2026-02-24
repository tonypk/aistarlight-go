package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

var (
	ErrJournalNotFound       = errors.New("journal entry not found")
	ErrJournalNotDraft       = errors.New("journal entry is not in draft status")
	ErrJournalNotPosted      = errors.New("journal entry is not posted")
	ErrJournalAlreadyReversed = errors.New("journal entry is already reversed")
	ErrUnbalancedEntry       = errors.New("debits must equal credits")
	ErrEmptyLines            = errors.New("journal entry must have at least 2 lines")
	ErrPeriodClosed          = errors.New("accounting period is closed")
)

type JournalService struct {
	q    *sqlc.Queries
	pool *pgxpool.Pool
}

func NewJournalService(q *sqlc.Queries, pool *pgxpool.Pool) *JournalService {
	return &JournalService{q: q, pool: pool}
}

type CreateJournalEntryInput struct {
	CompanyID   uuid.UUID
	EntryDate   time.Time
	Reference   *string
	Description *string
	SourceType  *string
	SourceID    *uuid.UUID
	Memo        *string
	CreatedBy   uuid.UUID
	Lines       []CreateJournalLineInput
}

type CreateJournalLineInput struct {
	AccountID   uuid.UUID
	Description *string
	Debit       decimal.Decimal
	Credit      decimal.Decimal

	// accountNumber is used internally by JournalGenerator for account resolution.
	// Not used by JournalService.Create directly.
	accountNumber string
}

// Create creates a journal entry with lines inside a DB transaction.
// Enforces: sum(debits) == sum(credits), at least 2 lines.
func (s *JournalService) Create(ctx context.Context, input CreateJournalEntryInput) (*domain.JournalEntry, error) {
	if len(input.Lines) < 2 {
		return nil, ErrEmptyLines
	}

	// Validate debit/credit balance
	totalDebit := decimal.Zero
	totalCredit := decimal.Zero
	for _, line := range input.Lines {
		totalDebit = totalDebit.Add(line.Debit)
		totalCredit = totalCredit.Add(line.Credit)
	}
	if !totalDebit.Equal(totalCredit) {
		return nil, fmt.Errorf("%w: debit=%s credit=%s", ErrUnbalancedEntry, totalDebit, totalCredit)
	}

	// Find open period for entry date
	periodID := pgtype.UUID{}
	period, err := s.q.FindPeriodByDate(ctx, sqlc.FindPeriodByDateParams{
		CompanyID: input.CompanyID,
		StartDate: pgtype.Date{Time: input.EntryDate, Valid: true},
	})
	if err == nil {
		periodID = pgtype.UUID{Bytes: period.ID, Valid: true}
	}
	// If no period found, allow creation without period (period_id is nullable)

	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	sourceID := pgtype.UUID{}
	if input.SourceID != nil {
		sourceID = pgtype.UUID{Bytes: *input.SourceID, Valid: true}
	}

	entryDate := pgtype.Date{Time: input.EntryDate, Valid: true}
	dbEntry, err := qtx.CreateJournalEntry(ctx, sqlc.CreateJournalEntryParams{
		ID:          uuid.New(),
		CompanyID:   input.CompanyID,
		PeriodID:    periodID,
		EntryDate:   entryDate,
		Reference:   input.Reference,
		Description: input.Description,
		SourceType:  input.SourceType,
		SourceID:    sourceID,
		Memo:        input.Memo,
		CreatedBy:   pgtype.UUID{Bytes: input.CreatedBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create journal entry: %w", err)
	}

	lines := make([]domain.JournalLine, len(input.Lines))
	for i, lineInput := range input.Lines {
		debitNum := pgtype.Numeric{}
		_ = debitNum.Scan(lineInput.Debit.String())
		creditNum := pgtype.Numeric{}
		_ = creditNum.Scan(lineInput.Credit.String())

		dbLine, err := qtx.CreateJournalLine(ctx, sqlc.CreateJournalLineParams{
			ID:             uuid.New(),
			JournalEntryID: dbEntry.ID,
			AccountID:      lineInput.AccountID,
			LineNumber:     int32(i + 1),
			Description:    lineInput.Description,
			Debit:          debitNum,
			Credit:         creditNum,
		})
		if err != nil {
			return nil, fmt.Errorf("create journal line %d: %w", i+1, err)
		}

		lines[i] = domain.JournalLine{
			ID:             dbLine.ID,
			JournalEntryID: dbLine.JournalEntryID,
			AccountID:      dbLine.AccountID,
			LineNumber:     int(dbLine.LineNumber),
			Description:    dbLine.Description,
			Debit:          lineInput.Debit,
			Credit:         lineInput.Credit,
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	entry := toJournalEntry(dbEntry)
	entry.Lines = lines
	return entry, nil
}

// GetByID retrieves a journal entry with its lines.
func (s *JournalService) GetByID(ctx context.Context, id uuid.UUID) (*domain.JournalEntry, error) {
	dbEntry, err := s.q.GetJournalEntryByID(ctx, id)
	if err != nil {
		return nil, ErrJournalNotFound
	}

	dbLines, err := s.q.ListJournalLinesByEntry(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list journal lines: %w", err)
	}

	entry := toJournalEntry(dbEntry)
	entry.Lines = make([]domain.JournalLine, len(dbLines))
	for i, l := range dbLines {
		entry.Lines[i] = domain.JournalLine{
			ID:             l.ID,
			JournalEntryID: l.JournalEntryID,
			AccountID:      l.AccountID,
			LineNumber:     int(l.LineNumber),
			Description:    l.Description,
			Debit:          numericToDecimal(l.Debit),
			Credit:         numericToDecimal(l.Credit),
			AccountName:    l.AccountName,
			AccountNumber:  l.AccountNumber,
		}
	}

	return entry, nil
}

// List returns paginated journal entries for a company.
func (s *JournalService) List(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.JournalEntry, int64, error) {
	entries, err := s.q.ListJournalEntries(ctx, sqlc.ListJournalEntriesParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list journal entries: %w", err)
	}

	count, err := s.q.CountJournalEntries(ctx, companyID)
	if err != nil {
		return nil, 0, fmt.Errorf("count journal entries: %w", err)
	}

	result := make([]domain.JournalEntry, len(entries))
	for i, e := range entries {
		result[i] = *toJournalEntry(e)
	}
	return result, count, nil
}

// Post transitions a journal entry from draft to posted.
func (s *JournalService) Post(ctx context.Context, id uuid.UUID, postedBy uuid.UUID) error {
	entry, err := s.q.GetJournalEntryByID(ctx, id)
	if err != nil {
		return ErrJournalNotFound
	}
	if entry.Status != string(domain.JournalStatusDraft) {
		return ErrJournalNotDraft
	}

	// Check period is open if assigned
	if entry.PeriodID.Valid {
		period, err := s.q.GetAccountingPeriodByID(ctx, entry.PeriodID.Bytes)
		if err == nil && period.Status != string(domain.PeriodOpen) {
			return ErrPeriodClosed
		}
	}

	return s.q.PostJournalEntry(ctx, sqlc.PostJournalEntryParams{
		ID:       id,
		PostedBy: pgtype.UUID{Bytes: postedBy, Valid: true},
	})
}

// Reverse creates a reversing journal entry for a posted entry.
func (s *JournalService) Reverse(ctx context.Context, id uuid.UUID, reversedBy uuid.UUID) (*domain.JournalEntry, error) {
	entry, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry.Status != domain.JournalStatusPosted {
		return nil, ErrJournalNotPosted
	}
	if entry.ReversedByID != nil {
		return nil, ErrJournalAlreadyReversed
	}

	// Build reversed lines (swap debit/credit)
	reversedLines := make([]CreateJournalLineInput, len(entry.Lines))
	for i, line := range entry.Lines {
		reversedLines[i] = CreateJournalLineInput{
			AccountID:   line.AccountID,
			Description: line.Description,
			Debit:       line.Credit, // swap
			Credit:      line.Debit,  // swap
		}
	}

	ref := fmt.Sprintf("Reversal of #%d", entry.EntryNumber)
	desc := fmt.Sprintf("Reversal of journal entry #%d", entry.EntryNumber)

	// Create reversing entry in a transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	periodID := pgtype.UUID{}
	if entry.PeriodID != nil {
		periodID = pgtype.UUID{Bytes: *entry.PeriodID, Valid: true}
	}
	entryDate := pgtype.Date{Time: time.Now(), Valid: true}

	// Create reversing entry
	reversingEntry, err := qtx.CreateJournalEntry(ctx, sqlc.CreateJournalEntryParams{
		ID:          uuid.New(),
		CompanyID:   entry.CompanyID,
		PeriodID:    periodID,
		EntryDate:   entryDate,
		Reference:   &ref,
		Description: &desc,
		SourceType:  entry.SourceType,
		SourceID:    pgtype.UUID{},
		Memo:        nil,
		CreatedBy:   pgtype.UUID{Bytes: reversedBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create reversing entry: %w", err)
	}

	// Set reverses_id on the new entry (not in CreateJournalEntry params, use raw SQL via update)
	// Actually, we need to set reversed_by_id on the original and reverses_id on the new one.
	// The generated queries use ReverseJournalEntry for the original entry.
	_, err = tx.Exec(ctx, "UPDATE journal_entries SET reverses_id = $1 WHERE id = $2", id, reversingEntry.ID)
	if err != nil {
		return nil, fmt.Errorf("set reverses_id: %w", err)
	}

	// Create reversed lines
	for i, lineInput := range reversedLines {
		debitNum := pgtype.Numeric{}
		_ = debitNum.Scan(lineInput.Debit.String())
		creditNum := pgtype.Numeric{}
		_ = creditNum.Scan(lineInput.Credit.String())

		_, err := qtx.CreateJournalLine(ctx, sqlc.CreateJournalLineParams{
			ID:             uuid.New(),
			JournalEntryID: reversingEntry.ID,
			AccountID:      lineInput.AccountID,
			LineNumber:     int32(i + 1),
			Description:    lineInput.Description,
			Debit:          debitNum,
			Credit:         creditNum,
		})
		if err != nil {
			return nil, fmt.Errorf("create reversing line %d: %w", i+1, err)
		}
	}

	// Post the reversing entry immediately
	err = qtx.PostJournalEntry(ctx, sqlc.PostJournalEntryParams{
		ID:       reversingEntry.ID,
		PostedBy: pgtype.UUID{Bytes: reversedBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("post reversing entry: %w", err)
	}

	// Mark original as reversed
	err = qtx.ReverseJournalEntry(ctx, sqlc.ReverseJournalEntryParams{
		ID:           id,
		ReversedByID: pgtype.UUID{Bytes: reversingEntry.ID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("mark original as reversed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return s.GetByID(ctx, reversingEntry.ID)
}

func toJournalEntry(e sqlc.JournalEntry) *domain.JournalEntry {
	je := &domain.JournalEntry{
		ID:          e.ID,
		CompanyID:   e.CompanyID,
		Status:      domain.JournalStatus(e.Status),
		Reference:   e.Reference,
		Description: e.Description,
		SourceType:  e.SourceType,
		Memo:        e.Memo,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
	if e.EntryNumber != nil {
		je.EntryNumber = int(*e.EntryNumber)
	}
	if e.EntryDate.Valid {
		je.EntryDate = e.EntryDate.Time
	}
	if e.PeriodID.Valid {
		id := uuid.UUID(e.PeriodID.Bytes)
		je.PeriodID = &id
	}
	if e.SourceID.Valid {
		id := uuid.UUID(e.SourceID.Bytes)
		je.SourceID = &id
	}
	if e.PostedBy.Valid {
		id := uuid.UUID(e.PostedBy.Bytes)
		je.PostedBy = &id
	}
	if e.PostedAt.Valid {
		t := e.PostedAt.Time
		je.PostedAt = &t
	}
	if e.ReversedByID.Valid {
		id := uuid.UUID(e.ReversedByID.Bytes)
		je.ReversedByID = &id
	}
	if e.ReversesID.Valid {
		id := uuid.UUID(e.ReversesID.Bytes)
		je.ReversesID = &id
	}
	if e.CreatedBy.Valid {
		id := uuid.UUID(e.CreatedBy.Bytes)
		je.CreatedBy = &id
	}
	return je
}

func numericToDecimal(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid {
		return decimal.Zero
	}
	// pgtype.Numeric stores Int and Exp
	val := decimal.NewFromBigInt(n.Int, n.Exp)
	return val
}

// Ensure pgx.Tx is imported for the pool Begin
var _ pgx.Tx = nil
