package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

// DetectedAnomaly represents an anomaly found during analysis.
type DetectedAnomaly struct {
	AnomalyType   string                 `json:"anomaly_type"`
	Severity      string                 `json:"severity"`
	Description   string                 `json:"description"`
	Details       map[string]interface{} `json:"details"`
	TransactionID *uuid.UUID             `json:"transaction_id,omitempty"`
}

// RunAnomalyDetection runs all anomaly detectors on session transactions.
func RunAnomalyDetection(transactions []map[string]interface{}) []DetectedAnomaly {
	var all []DetectedAnomaly

	all = append(all, detectDuplicates(transactions)...)
	all = append(all, detectVATMismatches(transactions)...)
	all = append(all, detectIncompleteTINs(transactions)...)
	all = append(all, detectUnusualAmounts(transactions)...)

	bankTxns := filterBySourceType(transactions, "bank_statement")
	recordTxns := filterNotSourceType(transactions, "bank_statement")
	if len(bankTxns) > 0 {
		all = append(all, detectMissingInvoices(bankTxns, recordTxns)...)
	}

	return all
}

func detectDuplicates(transactions []map[string]interface{}) []DetectedAnomaly {
	type group struct {
		txns []map[string]interface{}
	}
	seen := make(map[string]*group)

	for _, txn := range transactions {
		key := fmt.Sprintf("%s|%s|%s",
			toString(txn["date"]),
			toString(txn["amount"]),
			strings.ToLower(strings.TrimSpace(toString(txn["description"]))),
		)
		if _, ok := seen[key]; !ok {
			seen[key] = &group{}
		}
		seen[key].txns = append(seen[key].txns, txn)
	}

	var anomalies []DetectedAnomaly
	for _, g := range seen {
		if len(g.txns) <= 1 {
			continue
		}
		ids := make([]string, len(g.txns))
		for i, t := range g.txns {
			ids[i] = toString(t["id"])
		}
		var txnID *uuid.UUID
		if ids[0] != "" {
			if parsed, err := uuid.Parse(ids[0]); err == nil {
				txnID = &parsed
			}
		}
		desc := toString(g.txns[0]["description"])
		if len(desc) > 100 {
			desc = desc[:100]
		}
		anomalies = append(anomalies, DetectedAnomaly{
			AnomalyType: "duplicate",
			Severity:    "medium",
			Description: fmt.Sprintf("Possible duplicate: %d transactions with same date/amount/description", len(g.txns)),
			Details: map[string]interface{}{
				"transaction_ids": ids,
				"count":           len(g.txns),
				"date":            toString(g.txns[0]["date"]),
				"amount":          g.txns[0]["amount"],
				"description":     desc,
			},
			TransactionID: txnID,
		})
	}
	return anomalies
}

func detectVATMismatches(transactions []map[string]interface{}) []DetectedAnomaly {
	const vatRate = 0.12
	const tolerance = 0.02

	var anomalies []DetectedAnomaly
	for _, txn := range transactions {
		amount := parseAmount(txn["amount"])
		vatAmount := parseAmount(txn["vat_amount"])
		vatType := toString(txn["vat_type"])

		if vatType != "vatable" || amount <= 0 {
			continue
		}

		expectedVAT := amount * vatRate
		if vatAmount > 0 && math.Abs(vatAmount-expectedVAT) > expectedVAT*tolerance {
			var txnID *uuid.UUID
			if id := toString(txn["id"]); id != "" {
				if parsed, err := uuid.Parse(id); err == nil {
					txnID = &parsed
				}
			}
			anomalies = append(anomalies, DetectedAnomaly{
				AnomalyType: "vat_mismatch",
				Severity:    "high",
				Description: fmt.Sprintf("VAT mismatch: expected ~%.2f but found %.2f", expectedVAT, vatAmount),
				Details: map[string]interface{}{
					"amount":       amount,
					"vat_amount":   vatAmount,
					"expected_vat": math.Round(expectedVAT*100) / 100,
					"difference":   math.Round(math.Abs(vatAmount-expectedVAT)*100) / 100,
					"vat_type":     vatType,
				},
				TransactionID: txnID,
			})
		}
	}
	return anomalies
}

func detectIncompleteTINs(transactions []map[string]interface{}) []DetectedAnomaly {
	const threshold = 1000.0

	var anomalies []DetectedAnomaly
	for _, txn := range transactions {
		amount := parseAmount(txn["amount"])
		tin := strings.TrimSpace(strings.ToLower(toString(txn["tin"])))

		if amount >= threshold && (tin == "" || tin == "none" || tin == "nan") {
			var txnID *uuid.UUID
			if id := toString(txn["id"]); id != "" {
				if parsed, err := uuid.Parse(id); err == nil {
					txnID = &parsed
				}
			}
			desc := toString(txn["description"])
			if len(desc) > 100 {
				desc = desc[:100]
			}
			anomalies = append(anomalies, DetectedAnomaly{
				AnomalyType: "incomplete_tin",
				Severity:    "medium",
				Description: fmt.Sprintf("Missing TIN for transaction of %.2f PHP", amount),
				Details: map[string]interface{}{
					"amount":      amount,
					"description": desc,
					"date":        toString(txn["date"]),
				},
				TransactionID: txnID,
			})
		}
	}
	return anomalies
}

func detectUnusualAmounts(transactions []map[string]interface{}) []DetectedAnomaly {
	const zScoreThreshold = 3.0

	var amounts []float64
	for _, t := range transactions {
		a := parseAmount(t["amount"])
		if a > 0 {
			amounts = append(amounts, a)
		}
	}
	if len(amounts) < 5 {
		return nil
	}

	mean := 0.0
	for _, a := range amounts {
		mean += a
	}
	mean /= float64(len(amounts))

	variance := 0.0
	for _, a := range amounts {
		d := a - mean
		variance += d * d
	}
	stdev := math.Sqrt(variance / float64(len(amounts)-1))
	if stdev == 0 {
		return nil
	}

	var anomalies []DetectedAnomaly
	for _, txn := range transactions {
		amount := parseAmount(txn["amount"])
		if amount <= 0 {
			continue
		}
		zScore := (amount - mean) / stdev
		if math.Abs(zScore) < zScoreThreshold {
			continue
		}
		severity := "medium"
		if math.Abs(zScore) >= 5.0 {
			severity = "high"
		}
		var txnID *uuid.UUID
		if id := toString(txn["id"]); id != "" {
			if parsed, err := uuid.Parse(id); err == nil {
				txnID = &parsed
			}
		}
		desc := toString(txn["description"])
		if len(desc) > 100 {
			desc = desc[:100]
		}
		anomalies = append(anomalies, DetectedAnomaly{
			AnomalyType: "unusual_amount",
			Severity:    severity,
			Description: fmt.Sprintf("Unusual amount: %.2f PHP (z-score: %.1f)", amount, zScore),
			Details: map[string]interface{}{
				"amount":      amount,
				"z_score":     math.Round(zScore*100) / 100,
				"mean":        math.Round(mean*100) / 100,
				"stdev":       math.Round(stdev*100) / 100,
				"description": desc,
				"date":        toString(txn["date"]),
			},
			TransactionID: txnID,
		})
	}
	return anomalies
}

func detectMissingInvoices(bankTxns, recordTxns []map[string]interface{}) []DetectedAnomaly {
	const amountTolerance = 0.01
	const dateToleranceDays = 3

	matched := make([]bool, len(recordTxns))
	var anomalies []DetectedAnomaly

	for _, bank := range bankTxns {
		bankAmount := parseAmount(bank["amount"])
		found := false
		for j, rec := range recordTxns {
			if matched[j] {
				continue
			}
			recAmount := parseAmount(rec["amount"])
			tolerance := math.Max(amountTolerance, math.Abs(bankAmount)*0.001)
			if math.Abs(bankAmount-recAmount) > tolerance {
				continue
			}
			dateDiff := dateDiffDays(toString(bank["date"]), toString(rec["date"]))
			if dateDiff > dateToleranceDays {
				continue
			}
			matched[j] = true
			found = true
			break
		}
		if !found && bankAmount > 0 {
			txnType := toString(bank["type"])
			anomalyType := "unmatched_payment"
			if txnType == "credit" {
				anomalyType = "unmatched_deposit"
			}
			severity := "medium"
			if bankAmount >= 10000 {
				severity = "high"
			}
			var txnID *uuid.UUID
			if id := toString(bank["id"]); id != "" {
				if parsed, err := uuid.Parse(id); err == nil {
					txnID = &parsed
				}
			}
			desc := toString(bank["description"])
			if len(desc) > 100 {
				desc = desc[:100]
			}
			anomalies = append(anomalies, DetectedAnomaly{
				AnomalyType: anomalyType,
				Severity:    severity,
				Description: fmt.Sprintf("Bank %s of %.2f PHP has no matching record", txnType, bankAmount),
				Details: map[string]interface{}{
					"bank_amount":      bankAmount,
					"bank_date":        toString(bank["date"]),
					"bank_description": desc,
					"bank_reference":   toString(bank["reference"]),
				},
				TransactionID: txnID,
			})
		}
	}

	for j, rec := range recordTxns {
		if matched[j] {
			continue
		}
		recAmount := parseAmount(rec["amount"])
		if recAmount <= 0 {
			continue
		}
		var txnID *uuid.UUID
		if id := toString(rec["id"]); id != "" {
			if parsed, err := uuid.Parse(id); err == nil {
				txnID = &parsed
			}
		}
		desc := toString(rec["description"])
		if len(desc) > 100 {
			desc = desc[:100]
		}
		anomalies = append(anomalies, DetectedAnomaly{
			AnomalyType: "missing_invoice",
			Severity:    "medium",
			Description: fmt.Sprintf("Record of %.2f PHP has no matching bank transaction", recAmount),
			Details: map[string]interface{}{
				"record_amount":      recAmount,
				"record_date":        toString(rec["date"]),
				"record_description": desc,
			},
			TransactionID: txnID,
		})
	}

	return anomalies
}

func filterBySourceType(txns []map[string]interface{}, sourceType string) []map[string]interface{} {
	var result []map[string]interface{}
	for _, t := range txns {
		if toString(t["source_type"]) == sourceType {
			result = append(result, t)
		}
	}
	return result
}

func filterNotSourceType(txns []map[string]interface{}, sourceType string) []map[string]interface{} {
	var result []map[string]interface{}
	for _, t := range txns {
		if toString(t["source_type"]) != sourceType {
			result = append(result, t)
		}
	}
	return result
}
