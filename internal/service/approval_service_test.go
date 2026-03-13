package service

import (
	"math"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNumericToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    pgtype.Numeric
		expected float64
		valid    bool
	}{
		{
			name:     "simple integer",
			input:    pgtype.Numeric{Int: big.NewInt(10000), Exp: 0, Valid: true},
			expected: 10000,
			valid:    true,
		},
		{
			name:     "decimal value 100.50",
			input:    pgtype.Numeric{Int: big.NewInt(10050), Exp: -2, Valid: true},
			expected: 100.50,
			valid:    true,
		},
		{
			name:     "large threshold 50000",
			input:    pgtype.Numeric{Int: big.NewInt(50000), Exp: 0, Valid: true},
			expected: 50000,
			valid:    true,
		},
		{
			name:     "invalid numeric",
			input:    pgtype.Numeric{Valid: false},
			expected: 0,
			valid:    false,
		},
		{
			name:     "nil int",
			input:    pgtype.Numeric{Int: nil, Valid: true},
			expected: 0,
			valid:    false,
		},
		{
			name:     "positive exponent 5e3",
			input:    pgtype.Numeric{Int: big.NewInt(5), Exp: 3, Valid: true},
			expected: 5000,
			valid:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := numericToFloat64(tt.input)
			if ok != tt.valid {
				t.Errorf("valid = %v, want %v", ok, tt.valid)
			}
			if ok && math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("numericToFloat64 = %f, want %f", result, tt.expected)
			}
		})
	}
}

func TestApprovalCheckResult_Defaults(t *testing.T) {
	r := ApprovalCheckResult{}
	if r.NeedsApproval {
		t.Error("default ApprovalCheckResult should not need approval")
	}
	if r.TriggerReason != "" {
		t.Errorf("default trigger reason should be empty, got %q", r.TriggerReason)
	}
}
