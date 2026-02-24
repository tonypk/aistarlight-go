package service

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestGetEWTRate(t *testing.T) {
	tests := []struct {
		atcCode  string
		wantRate string
		wantErr  bool
	}{
		{"WI010", "0.05", false},
		{"WI020", "0.10", false},
		{"WC010", "0.10", false},
		{"WI030", "0.05", false},
		{"WI050", "0.02", false},
		{"WC050", "0.02", false},
		{"WI100", "0.01", false},
		{"WC100", "0.01", false},
		{"WI150", "0.02", false},
		// Case insensitive
		{"wi010", "0.05", false},
		{"Wi010", "0.05", false},
		// Unknown code
		{"XX999", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.atcCode, func(t *testing.T) {
			rate, err := GetEWTRate(tt.atcCode)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetEWTRate(%q) expected error, got nil", tt.atcCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetEWTRate(%q) unexpected error: %v", tt.atcCode, err)
			}
			want := decimal.RequireFromString(tt.wantRate)
			if !rate.Equal(want) {
				t.Errorf("GetEWTRate(%q) = %s, want %s", tt.atcCode, rate, want)
			}
		})
	}
}

func TestGetEWTIncomeType(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"WI010", "Professional fees - Individual <3M"},
		{"WC010", "Professional fees - Corporation"},
		{"XX999", "Other income"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := GetEWTIncomeType(tt.code)
			if got != tt.want {
				t.Errorf("GetEWTIncomeType(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestFindATCByKeywords(t *testing.T) {
	tests := []struct {
		desc         string
		supplierType string
		wantPrefix   string // Check prefix since map iteration is non-deterministic for tied scores
	}{
		// Individual professional → WI0xx
		{"professional consultant", "individual", "WI"},
		// Corporate professional → WC0xx
		{"professional consultant", "corporation", "WC"},
		// Rent → WI030 or WI040
		{"office rent lease", "individual", "WI"},
		// Contractor individual → WI050
		{"construction contractor", "individual", "WI050"},
		// Contractor corporation → WC050
		{"construction contractor", "corporation", "WC050"},
		// Commission individual → WI070
		{"agent commission", "individual", "WI070"},
		// Commission corporation → WC070
		{"agent commission broker", "corporation", "WC070"},
		// No match → empty
		{"random unrelated text", "individual", ""},
	}

	for _, tt := range tests {
		t.Run(tt.desc+"/"+tt.supplierType, func(t *testing.T) {
			got := FindATCByKeywords(tt.desc, tt.supplierType)
			if tt.wantPrefix == "" {
				if got != "" {
					t.Errorf("FindATCByKeywords(%q, %q) = %q, want empty", tt.desc, tt.supplierType, got)
				}
				return
			}
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("FindATCByKeywords(%q, %q) = %q, want prefix %q", tt.desc, tt.supplierType, got, tt.wantPrefix)
			}
		})
	}
}

func TestListEWTRates(t *testing.T) {
	rates := ListEWTRates()
	if len(rates) == 0 {
		t.Fatal("ListEWTRates() returned empty list")
	}
	if len(rates) != len(EWTRates) {
		t.Errorf("ListEWTRates() returned %d rates, want %d", len(rates), len(EWTRates))
	}

	// Verify all rates are non-negative
	for _, r := range rates {
		if r.Rate.LessThan(decimal.Zero) {
			t.Errorf("Rate for %s is negative: %s", r.ATCCode, r.Rate)
		}
		if r.ATCCode == "" {
			t.Error("Found rate with empty ATC code")
		}
	}
}
