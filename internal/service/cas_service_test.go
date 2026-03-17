package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestComputeJournalHash_Deterministic(t *testing.T) {
	companyID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	h1 := computeJournalHash(companyID, 1, date, "Test entry", "")
	h2 := computeJournalHash(companyID, 1, date, "Test entry", "")

	if h1 != h2 {
		t.Errorf("expected deterministic hash, got %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char SHA-256 hex, got %d chars", len(h1))
	}
}

func TestComputeJournalHash_DifferentInputs(t *testing.T) {
	companyID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	h1 := computeJournalHash(companyID, 1, date, "Entry A", "")
	h2 := computeJournalHash(companyID, 2, date, "Entry A", "")
	h3 := computeJournalHash(companyID, 1, date, "Entry B", "")
	h4 := computeJournalHash(companyID, 1, date, "Entry A", "someprevhash")

	hashes := []string{h1, h2, h3, h4}
	seen := map[string]bool{}
	for _, h := range hashes {
		if seen[h] {
			t.Errorf("duplicate hash found: %s", h)
		}
		seen[h] = true
	}
}

func TestComputeJournalHash_ChainDependency(t *testing.T) {
	companyID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	h1 := computeJournalHash(companyID, 1, date, "First entry", "")
	h2 := computeJournalHash(companyID, 2, date, "Second entry", h1)
	h3 := computeJournalHash(companyID, 3, date, "Third entry", h2)

	if h1 == h2 || h2 == h3 {
		t.Error("chained hashes should differ")
	}

	// Tampering with first entry should cascade
	h1alt := computeJournalHash(companyID, 1, date, "Tampered first", "")
	h2alt := computeJournalHash(companyID, 2, date, "Second entry", h1alt)
	if h2 == h2alt {
		t.Error("tampering with first entry should change second entry's hash")
	}
	_ = h3
}

func TestClassifyJournalBook(t *testing.T) {
	cash := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	revenue := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	expense := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	liability := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	ar := uuid.MustParse("55555555-5555-5555-5555-555555555555") // asset but not cash

	types := map[uuid.UUID]string{
		cash:      "asset",
		revenue:   "revenue",
		expense:   "expense",
		liability: "liability",
		ar:        "asset",
	}
	subTypes := map[uuid.UUID]string{
		cash: "cash",
		ar:   "accounts_receivable",
	}

	cases := []struct {
		name  string
		lines []CreateJournalLineInput
		want  string
	}{
		{
			name: "revenue entry → sales_journal",
			lines: []CreateJournalLineInput{
				{AccountID: cash, Debit: decimal.NewFromInt(1000)},
				{AccountID: revenue, Credit: decimal.NewFromInt(1000)},
			},
			want: JournalBookSales,
		},
		{
			name: "expense entry → purchases_journal",
			lines: []CreateJournalLineInput{
				{AccountID: expense, Debit: decimal.NewFromInt(500)},
				{AccountID: cash, Credit: decimal.NewFromInt(500)},
			},
			want: JournalBookPurchases,
		},
		{
			name: "cash debit only → cash_receipts",
			lines: []CreateJournalLineInput{
				{AccountID: cash, Debit: decimal.NewFromInt(1000)},
				{AccountID: liability, Credit: decimal.NewFromInt(1000)},
			},
			want: JournalBookCashReceipts,
		},
		{
			name: "cash credit only → cash_disbursements",
			lines: []CreateJournalLineInput{
				{AccountID: liability, Debit: decimal.NewFromInt(1000)},
				{AccountID: cash, Credit: decimal.NewFromInt(1000)},
			},
			want: JournalBookCashDisbursement,
		},
		{
			name: "non-cash asset (AR) → general_journal",
			lines: []CreateJournalLineInput{
				{AccountID: ar, Debit: decimal.NewFromInt(1000)},
				{AccountID: liability, Credit: decimal.NewFromInt(1000)},
			},
			want: JournalBookGeneral,
		},
		{
			name: "liability to liability → general_journal",
			lines: []CreateJournalLineInput{
				{AccountID: liability, Debit: decimal.NewFromInt(500)},
				{AccountID: liability, Credit: decimal.NewFromInt(500)},
			},
			want: JournalBookGeneral,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyJournalBook(tc.lines, types, subTypes)
			if got != tc.want {
				t.Errorf("ClassifyJournalBook() = %q, want %q", got, tc.want)
			}
		})
	}
}
