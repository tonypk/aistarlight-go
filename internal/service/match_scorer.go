package service

import (
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Common Philippine EWT (Expanded Withholding Tax) rates.
var ewtRates = [...]float64{0.01, 0.02, 0.05, 0.10, 0.15}

// MatchScore holds the multi-signal score for a record-bank entry pair.
type MatchScore struct {
	AmountScore      float64 `json:"amount_score"`      // 0-1, closer amounts = higher
	DateScore        float64 `json:"date_score"`         // 0-1, closer dates = higher
	ReferenceScore   float64 `json:"reference_score"`    // 0 or 1, reference number match
	DescriptionScore float64 `json:"description_score"`  // 0-1, keyword + edit distance
	Total            float64 `json:"total"`              // weighted total
	MatchType        string  `json:"match_type,omitempty"` // exact|fuzzy|net_of_tax|split
}

// SplitMatch represents a N:1 match where multiple records sum to one bank entry.
type SplitMatch struct {
	MatchGroupID uuid.UUID `json:"match_group_id"`
	RecordIDs    []string  `json:"record_ids"`
	BankID       string    `json:"bank_id"`
	RecordTotal  float64   `json:"record_total"`
	BankAmount   float64   `json:"bank_amount"`
	Difference   float64   `json:"difference"`
	MatchType    string    `json:"match_type,omitempty"`
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

	// Check EWT net-of-tax match: bank may have deducted withholding tax
	ewtScore, isEWT := scoreAmountEWT(recAmount, bankAmount)
	matchType := classifyMatch(amountScore, isEWT)

	// Use EWT amount score if it's better than the regular amount score
	effectiveAmountScore := amountScore
	if isEWT && ewtScore > amountScore {
		effectiveAmountScore = ewtScore
	}

	total := cfg.AmountWeight*effectiveAmountScore +
		cfg.DateWeight*dateScore +
		cfg.ReferenceWeight*refScore +
		cfg.DescriptionWeight*descScore

	return MatchScore{
		AmountScore:      effectiveAmountScore,
		DateScore:        dateScore,
		ReferenceScore:   refScore,
		DescriptionScore: descScore,
		Total:            total,
		MatchType:        matchType,
	}
}

// scoreAmountEWT checks if the bank amount equals the record amount minus a standard
// Philippine EWT rate (1%, 2%, 5%, 10%, 15%). Returns score and whether EWT was detected.
func scoreAmountEWT(recAmount, bankAmount float64) (float64, bool) {
	if recAmount <= 0 || bankAmount <= 0 || bankAmount >= recAmount {
		return 0, false
	}
	for _, rate := range ewtRates {
		expected := recAmount * (1 - rate)
		diff := math.Abs(bankAmount - expected)
		tolerance := math.Max(0.01, expected*0.001)
		if diff <= tolerance {
			return 1.0, true
		}
	}
	return 0, false
}

// classifyMatch determines the match type based on signal scores.
func classifyMatch(amountScore float64, isEWT bool) string {
	if isEWT {
		return "net_of_tax"
	}
	if amountScore >= 1.0 {
		return "exact"
	}
	return "fuzzy"
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

// DescriptionSimilarity computes a combined similarity score between two descriptions.
// It blends Jaccard keyword overlap (60%) with normalized Levenshtein distance (40%)
// for better fuzzy matching of bank descriptions vs. record descriptions.
func DescriptionSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}

	jaccard := jaccardSimilarity(a, b)
	levenshtein := levenshteinSimilarity(strings.ToLower(a), strings.ToLower(b))

	return 0.6*jaccard + 0.4*levenshtein
}

// jaccardSimilarity computes keyword overlap (Jaccard index) between two strings.
func jaccardSimilarity(a, b string) float64 {
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

// levenshteinSimilarity computes normalized edit distance similarity (0-1).
// Uses []rune for multibyte safety (Filipino/CJK descriptions) and O(min(m,n)) space.
func levenshteinSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}

	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 || lb == 0 {
		return 0
	}

	// Limit to first 200 runes to avoid O(n*m) explosion on very long descriptions
	if la > 200 {
		ra = ra[:200]
		la = 200
	}
	if lb > 200 {
		rb = rb[:200]
		lb = 200
	}

	// Ensure la <= lb for space efficiency
	if la > lb {
		ra, rb = rb, ra
		la, lb = lb, la
	}

	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := 0; i <= la; i++ {
		prev[i] = i
	}

	for j := 1; j <= lb; j++ {
		curr[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[i] = min(curr[i-1]+1, prev[i]+1, prev[i-1]+cost)
		}
		prev, curr = curr, prev
	}

	maxLen := math.Max(float64(la), float64(lb))
	return 1.0 - float64(prev[la])/maxLen
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
