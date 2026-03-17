package service

import (
	"testing"
)

func TestScoreMatch_ExactMatch(t *testing.T) {
	record := map[string]interface{}{
		"id":          "rec-1",
		"amount":      1000.0,
		"date":        "2024-01-15",
		"description": "ABC Supplier payment",
		"reference":   "INV-001",
	}
	bank := map[string]interface{}{
		"id":          "bank-1",
		"amount":      1000.0,
		"date":        "2024-01-15",
		"description": "ABC Supplier payment",
		"reference":   "INV-001",
	}
	cfg := DefaultScorerConfig()
	score := ScoreMatch(record, bank, cfg)

	if score.AmountScore != 1.0 {
		t.Errorf("expected amount score 1.0, got %f", score.AmountScore)
	}
	if score.DateScore != 1.0 {
		t.Errorf("expected date score 1.0, got %f", score.DateScore)
	}
	if score.ReferenceScore != 1.0 {
		t.Errorf("expected reference score 1.0, got %f", score.ReferenceScore)
	}
	if score.Total < 0.9 {
		t.Errorf("expected high total score, got %f", score.Total)
	}
}

func TestScoreMatch_AmountMismatch(t *testing.T) {
	record := map[string]interface{}{
		"id":     "rec-1",
		"amount": 1000.0,
		"date":   "2024-01-15",
	}
	bank := map[string]interface{}{
		"id":     "bank-1",
		"amount": 2000.0,
		"date":   "2024-01-15",
	}
	cfg := DefaultScorerConfig()
	score := ScoreMatch(record, bank, cfg)

	if score.AmountScore != 0 {
		t.Errorf("expected amount score 0 for large diff, got %f", score.AmountScore)
	}
}

func TestScoreMatch_DateProximity(t *testing.T) {
	record := map[string]interface{}{
		"id":     "rec-1",
		"amount": 1000.0,
		"date":   "2024-01-15",
	}
	bank := map[string]interface{}{
		"id":     "bank-1",
		"amount": 1000.0,
		"date":   "2024-01-18",
	}
	cfg := DefaultScorerConfig()
	score := ScoreMatch(record, bank, cfg)

	if score.DateScore >= 1.0 || score.DateScore <= 0 {
		t.Errorf("expected partial date score for 3-day diff, got %f", score.DateScore)
	}
}

func TestScoreMatch_ReferenceMatch(t *testing.T) {
	record := map[string]interface{}{
		"id":               "rec-1",
		"amount":           500.0,
		"date":             "2024-01-15",
		"invoice_number":   "12345",
	}
	bank := map[string]interface{}{
		"id":          "bank-1",
		"amount":      500.0,
		"date":        "2024-01-15",
		"description": "Payment for Invoice 12345",
	}
	cfg := DefaultScorerConfig()
	score := ScoreMatch(record, bank, cfg)

	if score.ReferenceScore < 0.8 {
		t.Errorf("expected reference score >= 0.8, got %f", score.ReferenceScore)
	}
}

func TestDescriptionSimilarity_Identical(t *testing.T) {
	score := DescriptionSimilarity("ABC Company Payment", "ABC Company Payment")
	if score != 1.0 {
		t.Errorf("expected 1.0 for identical descriptions, got %f", score)
	}
}

func TestDescriptionSimilarity_Overlap(t *testing.T) {
	score := DescriptionSimilarity("ABC Company Monthly Payment", "ABC Company Invoice")
	if score <= 0 {
		t.Errorf("expected positive score for overlapping descriptions, got %f", score)
	}
}

func TestDescriptionSimilarity_NoOverlap(t *testing.T) {
	score := DescriptionSimilarity("XYZ Corp", "Alpha Beta Gamma")
	// Jaccard gives 0 (no word overlap), but Levenshtein gives a small non-zero value
	// for character-level similarity. Score should be very low but not necessarily zero.
	if score > 0.10 {
		t.Errorf("expected very low score for completely different descriptions, got %f", score)
	}
}

func TestDescriptionSimilarity_Empty(t *testing.T) {
	if DescriptionSimilarity("", "something") != 0 {
		t.Error("expected 0 for empty string")
	}
	if DescriptionSimilarity("something", "") != 0 {
		t.Error("expected 0 for empty string")
	}
}

func TestExtractReferences_LabeledRef(t *testing.T) {
	refs := ExtractReferences("Payment for Invoice INV-2024-001 ref 12345")
	if len(refs) == 0 {
		t.Fatal("expected at least one reference extracted")
	}
	found := false
	for _, r := range refs {
		if r == "INV-2024-001" || r == "12345" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected INV-2024-001 or 12345 in refs, got %v", refs)
	}
}

func TestExtractReferences_DigitSequence(t *testing.T) {
	refs := ExtractReferences("Check deposit 98765432")
	found := false
	for _, r := range refs {
		if r == "98765432" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 98765432 in refs, got %v", refs)
	}
}

func TestExtractReferences_Empty(t *testing.T) {
	refs := ExtractReferences("")
	if len(refs) != 0 {
		t.Errorf("expected no refs for empty string, got %v", refs)
	}
}

func TestFindSplitMatches_TwoRecordsOneBank(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "rec-1", "amount": 300.0, "date": "2024-01-15"},
		{"id": "rec-2", "amount": 700.0, "date": "2024-01-15"},
		{"id": "rec-3", "amount": 500.0, "date": "2024-01-15"},
	}
	bank := map[string]interface{}{
		"id":     "bank-1",
		"amount": 1000.0,
		"date":   "2024-01-15",
	}

	splits := FindSplitMatches(records, bank, 0.01)
	if len(splits) == 0 {
		t.Fatal("expected at least one split match (300+700=1000)")
	}

	found := false
	for _, s := range splits {
		if s.BankAmount == 1000.0 && len(s.RecordIDs) == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected split match for 300+700=1000, got %+v", splits)
	}
}

func TestFindSplitMatches_NoMatch(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "rec-1", "amount": 300.0, "date": "2024-01-15"},
		{"id": "rec-2", "amount": 200.0, "date": "2024-01-15"},
	}
	bank := map[string]interface{}{
		"id":     "bank-1",
		"amount": 1000.0,
		"date":   "2024-01-15",
	}

	splits := FindSplitMatches(records, bank, 0.01)
	if len(splits) != 0 {
		t.Errorf("expected no split matches, got %d", len(splits))
	}
}

func TestFindSplitMatches_SingleRecord(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "rec-1", "amount": 1000.0, "date": "2024-01-15"},
	}
	bank := map[string]interface{}{
		"id":     "bank-1",
		"amount": 1000.0,
		"date":   "2024-01-15",
	}

	splits := FindSplitMatches(records, bank, 0.01)
	if len(splits) != 0 {
		t.Errorf("expected no split matches for single record, got %d", len(splits))
	}
}

func TestMatchTransactions_WithScoring(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "rec-1", "amount": 1000.0, "date": "2024-01-15", "description": "ABC Corp"},
		{"id": "rec-2", "amount": 2000.0, "date": "2024-01-16", "description": "XYZ Inc"},
		{"id": "rec-3", "amount": 500.0, "date": "2024-01-17", "description": "No Match"},
	}
	bankEntries := []map[string]interface{}{
		{"id": "bank-1", "amount": 1000.0, "date": "2024-01-15", "description": "ABC Corp payment"},
		{"id": "bank-2", "amount": 2000.0, "date": "2024-01-17", "description": "XYZ Inc transfer"},
		{"id": "bank-3", "amount": 9999.0, "date": "2024-01-18", "description": "Unrelated"},
	}

	result := MatchTransactions(records, bankEntries, 0.01, 3)

	if len(result.MatchedPairs) != 2 {
		t.Errorf("expected 2 matched pairs, got %d", len(result.MatchedPairs))
	}
	if len(result.UnmatchedRecords) != 1 {
		t.Errorf("expected 1 unmatched record, got %d", len(result.UnmatchedRecords))
	}
	if len(result.UnmatchedBank) != 1 {
		t.Errorf("expected 1 unmatched bank, got %d", len(result.UnmatchedBank))
	}

	// Check scores are attached
	for _, pair := range result.MatchedPairs {
		if pair.Score == nil {
			t.Error("expected Score to be populated on matched pair")
		}
	}
}

func TestMatchTransactions_SplitMatch(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "rec-1", "amount": 300.0, "date": "2024-01-15"},
		{"id": "rec-2", "amount": 700.0, "date": "2024-01-15"},
	}
	bankEntries := []map[string]interface{}{
		{"id": "bank-1", "amount": 1000.0, "date": "2024-01-15"},
	}

	result := MatchTransactions(records, bankEntries, 0.01, 3)

	// Should find a split match since neither record matches exactly
	if len(result.SplitMatches) != 1 {
		t.Errorf("expected 1 split match, got %d", len(result.SplitMatches))
	}
	if len(result.SplitMatches) > 0 {
		sm := result.SplitMatches[0]
		if len(sm.RecordIDs) != 2 {
			t.Errorf("expected 2 record IDs in split, got %d", len(sm.RecordIDs))
		}
	}
}

func TestTokenize(t *testing.T) {
	words := tokenize("ABC Company, Inc. - Monthly Payment")
	for _, w := range words {
		if stopWords[w] {
			t.Errorf("stop word %q should have been removed", w)
		}
	}
	expected := map[string]bool{"abc": true, "company": true, "monthly": true, "payment": true}
	for _, exp := range []string{"abc", "company", "monthly", "payment"} {
		found := false
		for _, w := range words {
			if w == exp {
				found = true
				break
			}
		}
		if !found && expected[exp] {
			t.Errorf("expected word %q not found in tokenized output", exp)
		}
	}
}

func TestTrackBalance(t *testing.T) {
	txns := []map[string]interface{}{
		{"amount": 5000.0, "source_type": "sales_record"},
		{"amount": 2000.0, "source_type": "purchase_record"},
		{"amount": 1000.0, "source_type": "bank_statement", "type": "credit"},
		{"amount": 500.0, "source_type": "bank_statement", "type": "debit"},
	}

	result := TrackBalance(10000.0, 0, txns)

	expectedCredits := 6000.0 // 5000 sales + 1000 bank credit
	expectedDebits := 2500.0  // 2000 purchases + 500 bank debit
	expectedClosing := 10000.0 + expectedCredits - expectedDebits // 13500

	if result.TotalCredits != expectedCredits {
		t.Errorf("expected total credits %.2f, got %.2f", expectedCredits, result.TotalCredits)
	}
	if result.TotalDebits != expectedDebits {
		t.Errorf("expected total debits %.2f, got %.2f", expectedDebits, result.TotalDebits)
	}
	if result.ComputedClosing != expectedClosing {
		t.Errorf("expected computed closing %.2f, got %.2f", expectedClosing, result.ComputedClosing)
	}
}

func TestTrackBalance_WithBankClosing(t *testing.T) {
	txns := []map[string]interface{}{
		{"amount": 5000.0, "source_type": "sales_record"},
	}

	result := TrackBalance(10000.0, 14000.0, txns)

	if result.IsBalanced {
		t.Error("expected not balanced since computed (15000) != bank (14000)")
	}
	if result.BalanceDifference != 1000.0 {
		t.Errorf("expected balance difference 1000, got %.2f", result.BalanceDifference)
	}
	if len(result.Discrepancies) == 0 {
		t.Error("expected discrepancy for balance mismatch")
	}
}
