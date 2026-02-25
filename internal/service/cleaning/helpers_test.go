package cleaning

import (
	"math"
	"testing"
)

func TestIsEmptyRow(t *testing.T) {
	tests := []struct {
		name string
		row  []string
		want bool
	}{
		{"all empty", []string{"", "", ""}, true},
		{"all spaces", []string{"  ", "\t", " "}, true},
		{"nil row", nil, true},
		{"has value", []string{"", "hello", ""}, false},
		{"number value", []string{"", "123", ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEmptyRow(tt.row); got != tt.want {
				t.Errorf("IsEmptyRow(%v) = %v, want %v", tt.row, got, tt.want)
			}
		})
	}
}

func TestIsSequentialNumbers(t *testing.T) {
	tests := []struct {
		name string
		row  []string
		want bool
	}{
		{"1,2,3,4", []string{"1", "2", "3", "4"}, true},
		{"(1),(2),(3)", []string{"(1)", "(2)", "(3)"}, true},
		{"with blanks", []string{"1", "", "2", "3"}, true},
		{"not sequential", []string{"1", "3", "5"}, false},
		{"too few", []string{"1", "2"}, false},
		{"has text", []string{"1", "two", "3"}, false},
		{"BIR numbering", []string{"(1)", "(2)", "(3)", "(4)", "(5)"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSequentialNumbers(tt.row); got != tt.want {
				t.Errorf("IsSequentialNumbers(%v) = %v, want %v", tt.row, got, tt.want)
			}
		})
	}
}

func TestLooksNumeric(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"12345", true},
		{"1,234.56", true},
		{"₱5,000.00", true},
		{"(1,234.56)", true},
		{"-500", true},
		{"01-15-2024", true},
		{"123-456-789-000", true},
		{"Hello World", false},
		{"Invoice No.", false},
		{"", false},
		{"ABC", false},
		{"100%", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := LooksNumeric(tt.input); got != tt.want {
				t.Errorf("LooksNumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFillRatio(t *testing.T) {
	tests := []struct {
		name      string
		row       []string
		totalCols int
		want      float64
	}{
		{"all filled", []string{"a", "b", "c"}, 3, 1.0},
		{"half filled", []string{"a", "", "c", ""}, 4, 0.5},
		{"none filled", []string{"", "", ""}, 3, 0.0},
		{"zero cols", []string{"a"}, 0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FillRatio(tt.row, tt.totalCols)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("FillRatio = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestTextRatio(t *testing.T) {
	tests := []struct {
		name string
		row  []string
		want float64
	}{
		{"all text", []string{"Name", "Address", "Status"}, 1.0},
		{"all numeric", []string{"123", "456", "789"}, 0.0},
		{"mixed", []string{"Name", "123", "Address", "456"}, 0.5},
		{"empty", []string{"", "", ""}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TextRatio(tt.row)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("TextRatio = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want float64
	}{
		{"identical", []string{"A", "B", "C"}, []string{"A", "B", "C"}, 1.0},
		{"no overlap", []string{"A", "B"}, []string{"C", "D"}, 0.0},
		{"half overlap", []string{"A", "B"}, []string{"A", "C"}, 1.0 / 3.0},
		{"case insensitive", []string{"Name", "TIN"}, []string{"name", "tin"}, 1.0},
		{"both empty", []string{}, []string{}, 1.0},
		{"one empty", []string{"A"}, []string{}, 0.0},
		{"with blanks", []string{"A", "", "B"}, []string{"A", "B", ""}, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JaccardSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("JaccardSimilarity = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestHeaderSignature(t *testing.T) {
	cols := []string{"Name", "TIN", "", "Column_3", "Amount"}
	sig := HeaderSignature(cols)
	// Should be sorted, lowercase, no empties, no Column_N
	expected := []string{"amount", "name", "tin"}
	if len(sig) != len(expected) {
		t.Fatalf("HeaderSignature got %v, want %v", sig, expected)
	}
	for i, s := range sig {
		if s != expected[i] {
			t.Errorf("HeaderSignature[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

func TestHeaderSignatureHash(t *testing.T) {
	cols1 := []string{"Name", "TIN", "Amount"}
	cols2 := []string{"Amount", "Name", "TIN"} // same columns, different order
	cols3 := []string{"Name", "TIN", "Date"}   // different

	hash1 := HeaderSignatureHash(cols1)
	hash2 := HeaderSignatureHash(cols2)
	hash3 := HeaderSignatureHash(cols3)

	if hash1 != hash2 {
		t.Errorf("Same columns different order should produce same hash")
	}
	if hash1 == hash3 {
		t.Errorf("Different columns should produce different hash")
	}
	if len(hash1) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars, got %d", len(hash1))
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
		ok    bool
	}{
		{"1234.56", 1234.56, true},
		{"1,234.56", 1234.56, true},
		{"₱5,000.00", 5000.00, true},
		{"(1,234.56)", -1234.56, true},
		{"-500", -500.0, true},
		{"0", 0, true},
		{"", 0, false},
		{"abc", 0, false},
		{"PHP 1000", 1000, true},
		{"-", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseFloat(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseFloat(%q) ok = %v, want %v", tt.input, ok, tt.ok)
				return
			}
			if ok && math.Abs(got-tt.want) > 0.001 {
				t.Errorf("ParseFloat(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestRowPreview(t *testing.T) {
	row := []string{"2024-01-15", "Total Logistics Inc.", "", "123-456-789-000", "5000.00"}
	preview := RowPreview(row)
	if len(preview) > 100 {
		t.Errorf("RowPreview should be <= 100 chars, got %d", len(preview))
	}
	if preview == "" {
		t.Error("RowPreview should not be empty for non-empty row")
	}
}

func TestFormatRowForAI(t *testing.T) {
	row := []string{"Hello", "", "World"}
	got := FormatRowForAI(row)
	if got != `["Hello", "", "World"]` {
		t.Errorf("FormatRowForAI = %q", got)
	}
}
