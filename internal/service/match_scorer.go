package service

import (
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// MatchScore holds the multi-signal score for a record-bank entry pair.
type MatchScore struct {
	AmountScore      float64 `json:"amount_score"`      // 0-1, closer amounts = higher
	DateScore        float64 `json:"date_score"`         // 0-1, closer dates = higher
	ReferenceScore   float64 `json:"reference_score"`    // 0 or 1, reference number match
	DescriptionScore float64 `json:"description_score"`  // 0-1, keyword overlap
	Total            float64 `json:"total"`              // weighted total
}

// SplitMatch represents a N:1 match where multiple records sum to one bank entry.
type SplitMatch struct {
	MatchGroupID uuid.UUID `json:"match_group_id"`
	RecordIDs    []string  `json:"record_ids"`
	BankID       string    `json:"bank_id"`
	RecordTotal  float64   `json:"record_total"`
	BankAmount   float64   `json:"bank_amount"`
	Difference   float64   `json:"difference"`
}

// ScorerConfig holds weights and tolerances for the scoring engine.
type ScorerConfig struct {
	AmountWeight      float64
	DateWeight        float64
	ReferenceWeight   float64
	DescriptionWeight float64
	AmountTolerance   float64
	DateToleranceDays int
	MinMatchScore     float64 // minimum total score to consider a match
}

// DefaultScorerConfig returns sensible defaults for Philippine CPA reconciliation.
func DefaultScorerConfig() ScorerConfig {
	return ScorerConfig{
		AmountWeight:      0.40,
		DateWeight:        0.20,
		ReferenceWeight:   0.25,
		DescriptionWeight: 0.15,
		AmountTolerance:   0.01,
		DateToleranceDays: 7,
		MinMatchScore:     0.50,
	}
}

// ScoreMatch computes a multi-signal match score between a record and a bank entry.
func ScoreMatch(record, bankEntry map[string]interface{}, cfg ScorerConfig) MatchScore {
	recAmount := parseAmount(record["amount"])
	bankAmount := parseAmount(bankEntry["amount"])
	recDate := toString(record["date"])
	bankDate := toString(bankEntry["date"])

	amountScore := scoreAmount(recAmount, bankAmount, cfg.AmountTolerance)
	dateScore := scoreDate(recDate, bankDate, cfg.DateToleranceDays)
	refScore := scoreReference(record, bankEntry)
	descScore := DescriptionSimilarity(toString(record["description"]), toString(bankEntry["description"]))

	total := cfg.AmountWeight*amountScore +
		cfg.DateWeight*dateScore +
		cfg.ReferenceWeight*refScore +
		cfg.DescriptionWeight*descScore

	return MatchScore{
		AmountScore:      amountScore,
		DateScore:        dateScore,
		ReferenceScore:   refScore,
		DescriptionScore: descScore,
		Total:            total,
	}
}

// FindSplitMatches finds groups of records whose amounts sum to a single bank entry.
// tolerance is the maximum allowed difference between the sum and the bank amount.
func FindSplitMatches(records []map[string]interface{}, bankEntry map[string]interface{}, tolerance float64) []SplitMatch {
	bankAmount := parseAmount(bankEntry["amount"])
	if bankAmount <= 0 {
		return nil
	}
	bankID := toString(bankEntry["id"])
	bankDate := toString(bankEntry["date"])

	if tolerance <= 0 {
		tolerance = math.Max(0.01, bankAmount*0.001)
	}

	// Filter records to only those within date range and individually smaller than bank amount
	type candidate struct {
		idx    int
		amount float64
		id     string
	}
	var candidates []candidate
	for i, rec := range records {
		recAmount := parseAmount(rec["amount"])
		if recAmount <= 0 || recAmount > bankAmount+tolerance {
			continue
		}
		recDate := toString(rec["date"])
		if recDate != "" && bankDate != "" && dateDiffDays(recDate, bankDate) > 14 {
			continue
		}
		candidates = append(candidates, candidate{idx: i, amount: recAmount, id: toString(rec["id"])})
	}

	if len(candidates) < 2 {
		return nil
	}

	var results []SplitMatch

	// Try pairs first (most common: 2 records = 1 bank entry)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			sum := candidates[i].amount + candidates[j].amount
			diff := math.Abs(sum - bankAmount)
			if diff <= tolerance {
				results = append(results, SplitMatch{
					MatchGroupID: uuid.New(),
					RecordIDs:    []string{candidates[i].id, candidates[j].id},
					BankID:       bankID,
					RecordTotal:  sum,
					BankAmount:   bankAmount,
					Difference:   diff,
				})
			}
		}
	}

	// Try triples (3 records = 1 bank entry) — only if no pairs found
	if len(results) == 0 && len(candidates) >= 3 && len(candidates) <= 20 {
		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				for k := j + 1; k < len(candidates); k++ {
					sum := candidates[i].amount + candidates[j].amount + candidates[k].amount
					diff := math.Abs(sum - bankAmount)
					if diff <= tolerance {
						results = append(results, SplitMatch{
							MatchGroupID: uuid.New(),
							RecordIDs:    []string{candidates[i].id, candidates[j].id, candidates[k].id},
							BankID:       bankID,
							RecordTotal:  sum,
							BankAmount:   bankAmount,
							Difference:   diff,
						})
					}
				}
			}
		}
	}

	return results
}

// ExtractReferences extracts potential reference numbers from a description string.
// Looks for check numbers, invoice numbers, reference numbers, and digit sequences.
func ExtractReferences(description string) []string {
	if description == "" {
		return nil
	}

	var refs []string
	seen := make(map[string]bool)

	// Pattern: explicit labeled references (e.g., "Ref: 12345", "Check No. 789", "INV-2024-001")
	labelPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:ref(?:erence)?|chk|check|inv(?:oice)?|or|cr|dr)[\s.:#-]*([A-Z0-9][\w-]{2,})`),
		regexp.MustCompile(`(?i)(?:no|num|number)[\s.:#-]*([A-Z0-9][\w-]{2,})`),
	}
	for _, pat := range labelPatterns {
		matches := pat.FindAllStringSubmatch(description, -1)
		for _, m := range matches {
			ref := strings.TrimSpace(m[1])
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}

	// Pattern: standalone digit sequences (5+ digits, likely reference numbers)
	digitPattern := regexp.MustCompile(`\b(\d{5,})\b`)
	matches := digitPattern.FindAllStringSubmatch(description, -1)
	for _, m := range matches {
		ref := m[1]
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}

	return refs
}

// DescriptionSimilarity computes keyword overlap (Jaccard-like) between two descriptions.
func DescriptionSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}

	wordsA := tokenize(a)
	wordsB := tokenize(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}
	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}

	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// --- internal helpers ---

func scoreAmount(recAmount, bankAmount, tolerance float64) float64 {
	if recAmount == 0 && bankAmount == 0 {
		return 1.0
	}
	if recAmount == 0 || bankAmount == 0 {
		return 0
	}

	diff := math.Abs(recAmount - bankAmount)
	maxAmt := math.Max(math.Abs(recAmount), math.Abs(bankAmount))
	effectiveTolerance := math.Max(tolerance, maxAmt*0.001)

	if diff <= effectiveTolerance {
		return 1.0 // exact match within tolerance
	}

	// Decay: score drops as difference grows, reaching 0 at 10% difference
	ratio := diff / maxAmt
	if ratio >= 0.10 {
		return 0
	}
	return 1.0 - (ratio / 0.10)
}

func scoreDate(dateA, dateB string, toleranceDays int) float64 {
	if dateA == "" || dateB == "" {
		return 0.5 // neutral when date missing
	}

	days := dateDiffDays(dateA, dateB)
	if days == 0 {
		return 1.0
	}
	if days > toleranceDays {
		return 0
	}
	return 1.0 - float64(days)/float64(toleranceDays)
}

func scoreReference(record, bankEntry map[string]interface{}) float64 {
	// Extract references from both sides
	recRefs := collectReferences(record)
	bankRefs := collectReferences(bankEntry)

	if len(recRefs) == 0 || len(bankRefs) == 0 {
		return 0 // no reference info to compare
	}

	for _, rr := range recRefs {
		for _, br := range bankRefs {
			if strings.EqualFold(rr, br) {
				return 1.0
			}
			// Check if one contains the other (partial match)
			if len(rr) >= 4 && len(br) >= 4 {
				if strings.Contains(strings.ToLower(rr), strings.ToLower(br)) ||
					strings.Contains(strings.ToLower(br), strings.ToLower(rr)) {
					return 0.8
				}
			}
		}
	}
	return 0
}

func collectReferences(entry map[string]interface{}) []string {
	var refs []string

	// Direct reference fields
	for _, key := range []string{"reference", "reference_number", "invoice_number", "check_number", "or_number"} {
		if v := toString(entry[key]); v != "" {
			refs = append(refs, v)
		}
	}

	// Extract from description
	if desc := toString(entry["description"]); desc != "" {
		refs = append(refs, ExtractReferences(desc)...)
	}

	return refs
}

// stopWords are common words to exclude from similarity comparison.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"of": true, "to": true, "in": true, "for": true, "on": true,
	"at": true, "by": true, "from": true, "with": true, "as": true,
	"is": true, "it": true, "this": true, "that": true, "no": true,
	"sa": true, "ng": true, "na": true, "ang": true, "mga": true, // Filipino stop words
	"php": true, "inc": true, "corp": true, "co": true, "ltd": true,
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	// Replace non-alphanumeric with spaces
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, s)

	words := strings.Fields(cleaned)
	var result []string
	for _, w := range words {
		if len(w) >= 2 && !stopWords[w] {
			result = append(result, w)
		}
	}
	return result
}
