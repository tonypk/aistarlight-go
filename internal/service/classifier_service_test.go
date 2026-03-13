package service

import (
	"testing"

	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

func TestApplyRuleBasedClassification_Government(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tx := map[string]interface{}{
		"description": "BIR payment for annual registration",
		"tin":         "",
		"source_type": "purchase_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r == nil {
		t.Fatal("expected rule match for government entity (BIR keyword)")
	}
	if r.VATType != "government" {
		t.Errorf("expected vat_type=government, got %s", r.VATType)
	}
	if r.Category != "goods" {
		t.Errorf("expected category=goods for purchase_record, got %s", r.Category)
	}
	if r.ClassificationSource != "rule" {
		t.Errorf("expected source=rule, got %s", r.ClassificationSource)
	}
}

func TestApplyRuleBasedClassification_GovernmentSales(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tx := map[string]interface{}{
		"description": "SSS contribution remittance",
		"tin":         "",
		"source_type": "sales_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r == nil {
		t.Fatal("expected rule match for government entity (SSS keyword)")
	}
	if r.Category != "sale" {
		t.Errorf("expected category=sale for sales_record, got %s", r.Category)
	}
}

func TestApplyRuleBasedClassification_Export(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tx := map[string]interface{}{
		"description": "Export shipment to Japan",
		"tin":         "",
		"source_type": "sales_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r == nil {
		t.Fatal("expected rule match for export")
	}
	if r.VATType != "zero_rated" {
		t.Errorf("expected vat_type=zero_rated, got %s", r.VATType)
	}
	if r.Category != "sale" {
		t.Errorf("expected category=sale for export sales, got %s", r.Category)
	}
}

func TestApplyRuleBasedClassification_Exempt(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tx := map[string]interface{}{
		"description": "Agricultural supplies purchase",
		"tin":         "",
		"source_type": "purchase_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r == nil {
		t.Fatal("expected rule match for exempt")
	}
	if r.VATType != "exempt" {
		t.Errorf("expected vat_type=exempt, got %s", r.VATType)
	}
	if r.Category != "goods" {
		t.Errorf("expected category=goods for exempt items, got %s", r.Category)
	}
}

func TestApplyRuleBasedClassification_NoMatch(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tx := map[string]interface{}{
		"description": "Office supplies from National Bookstore",
		"tin":         "",
		"source_type": "purchase_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r != nil {
		t.Errorf("expected no rule match for generic purchase, got %+v", r)
	}
}

func TestApplyRuleBasedClassification_PEZA(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tx := map[string]interface{}{
		"description": "PEZA registered enterprise sales",
		"tin":         "",
		"source_type": "sales_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r == nil {
		t.Fatal("expected rule match for PEZA")
	}
	if r.VATType != "zero_rated" {
		t.Errorf("expected vat_type=zero_rated, got %s", r.VATType)
	}
}

func TestApplyRuleBasedClassification_GovernmentTINPrefix(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	// PH government TIN prefix check (if configured in jurisdiction)
	if len(jCfg.GovtTINPrefixes) == 0 {
		t.Skip("no government TIN prefixes configured for PH")
	}
	tx := map[string]interface{}{
		"description": "Regular vendor",
		"tin":         jCfg.GovtTINPrefixes[0] + "12345",
		"source_type": "purchase_record",
	}
	r := applyRuleBasedClassification(tx, jCfg)
	if r == nil {
		t.Fatal("expected rule match for government TIN prefix")
	}
	if r.VATType != "government" {
		t.Errorf("expected vat_type=government, got %s", r.VATType)
	}
}

func TestBatchIndices(t *testing.T) {
	tests := []struct {
		name      string
		indices   []int
		batchSize int
		expected  int // number of batches
	}{
		{"empty", nil, 5, 0},
		{"exact batch", []int{0, 1, 2, 3, 4}, 5, 1},
		{"two batches", []int{0, 1, 2, 3, 4, 5}, 5, 2},
		{"small batch", []int{0, 1, 2}, 5, 1},
		{"many batches", make([]int, 26), 10, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches := batchIndices(tt.indices, tt.batchSize)
			if len(batches) != tt.expected {
				t.Errorf("batchIndices(%d items, size %d) = %d batches, want %d",
					len(tt.indices), tt.batchSize, len(batches), tt.expected)
			}
			// Verify all indices are covered.
			var total int
			for _, b := range batches {
				total += len(b)
			}
			if total != len(tt.indices) {
				t.Errorf("total indices in batches = %d, want %d", total, len(tt.indices))
			}
		})
	}
}

func TestIsGovernmentEntity(t *testing.T) {
	jCfg := jurisdiction.Get("PH")
	tests := []struct {
		desc     string
		tin      string
		expected bool
	}{
		{"payment to bir office", "", true},
		{"sss contribution", "", true},
		{"philhealth premium", "", true},
		{"pag-ibig fund", "", true},
		{"municipality of makati", "", true},
		{"jollibee food corp", "", false},
		{"sm supermarket", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := isGovernmentEntity(tt.desc, tt.tin, jCfg)
			if got != tt.expected {
				t.Errorf("isGovernmentEntity(%q, %q) = %v, want %v", tt.desc, tt.tin, got, tt.expected)
			}
		})
	}
}
