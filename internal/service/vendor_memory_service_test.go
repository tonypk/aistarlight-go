package service

import (
	"testing"
)

func TestNormalizeVendor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"spaces only", "   ", ""},
		{"simple lowercase", "ABC Store", "abc store"},
		{"strip Inc", "Jollibee Inc.", "jollibee"},
		{"strip Corp", "SM Corp", "sm"},
		{"strip LLC", "Tech Solutions LLC", "tech solutions"},
		{"strip Pte Ltd", "Grab Pte. Ltd.", "grab"},
		{"strip Co Ltd", "Toyota Co. Ltd.", "toyota"},
		{"strip Limited", "PLDT Limited", "pldt"},
		{"strip Corporation", "Globe Corporation", "globe"},
		{"strip Sdn Bhd", "Maybank Sdn Bhd", "maybank"},
		{"strip payment prefix SQ", "SQ *Coffee Shop", "coffee shop"},
		{"strip payment prefix Grab", "Grab *FoodPanda", "foodpanda"},
		{"strip payment prefix PayPal", "PayPal *Netflix", "netflix"},
		{"strip payment prefix Stripe", "Stripe *SaaS Product", "saas product"},
		{"strip payment prefix GOOG", "GOOG *YouTube Premium", "youtube premium"},
		{"strip payment prefix AMZN", "AMZN *Amazon Web Services", "amazon web services"},
		{"multiple spaces", "  SM   Super  Mall  ", "sm super mall"},
		{"combined suffix and spaces", "  Globe  Telecom  Inc.  ", "globe telecom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeVendor(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeVendor(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeVendor_Idempotent(t *testing.T) {
	input := "Jollibee Inc."
	first := NormalizeVendor(input)
	second := NormalizeVendor(first)
	if first != second {
		t.Errorf("NormalizeVendor is not idempotent: first=%q, second=%q", first, second)
	}
}
