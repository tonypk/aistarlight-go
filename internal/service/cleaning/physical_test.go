package cleaning

import (
	"testing"
)

func TestDetectDataRegions_SingleBlock(t *testing.T) {
	rows := CellGrid{
		{"", "", ""},           // blank
		{"Name", "TIN", "Amt"}, // header
		{"A", "123", "100"},    // data
		{"B", "456", "200"},    // data
		{"C", "789", "300"},    // data
		{"", "", ""},           // blank
	}
	regions := DetectDataRegions(rows)
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(regions))
	}
	r := regions[0]
	if r.StartRow != 1 || r.EndRow != 4 {
		t.Errorf("region = [%d, %d], want [1, 4]", r.StartRow, r.EndRow)
	}
	if r.RowCount != 4 {
		t.Errorf("RowCount = %d, want 4", r.RowCount)
	}
}

func TestDetectDataRegions_MultipleBlocks(t *testing.T) {
	rows := CellGrid{
		{"Summary", "", ""},
		{"Total", "10000", ""},
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},  // 3+ blank rows = separate regions
		{"Name", "TIN", "Amount"},
		{"A", "123", "100"},
		{"B", "456", "200"},
		{"C", "789", "300"},
		{"D", "012", "400"},
		{"E", "345", "500"},
	}
	regions := DetectDataRegions(rows)
	if len(regions) < 2 {
		t.Fatalf("expected >= 2 regions, got %d", len(regions))
	}
	best := BestDataRegion(regions)
	if best == nil {
		t.Fatal("BestDataRegion returned nil")
	}
	// The largest region should be the data block (6 rows)
	if best.RowCount < 5 {
		t.Errorf("best region RowCount = %d, expected >= 5", best.RowCount)
	}
}

func TestDetectDataRegions_AllEmpty(t *testing.T) {
	rows := CellGrid{
		{"", ""},
		{"", ""},
	}
	regions := DetectDataRegions(rows)
	if len(regions) != 0 {
		t.Errorf("expected 0 regions for all-empty, got %d", len(regions))
	}
}

func TestPruneEmptyColumns(t *testing.T) {
	rows := CellGrid{
		{"A", "", "B", "", "C"},
		{"D", "", "E", "", "F"},
		{"G", "", "H", "", "I"},
	}
	empty := PruneEmptyColumns(rows)
	if len(empty) != 2 {
		t.Fatalf("expected 2 empty columns, got %d: %v", len(empty), empty)
	}
	if empty[0] != 1 || empty[1] != 3 {
		t.Errorf("empty columns = %v, want [1, 3]", empty)
	}
}

func TestPruneEmptyRows(t *testing.T) {
	rows := CellGrid{
		{"", ""},
		{"", ""},
		{"A", "B"},
		{"C", "D"},
		{"", ""},
	}
	leading, trailing := PruneEmptyRows(rows)
	if leading != 2 {
		t.Errorf("leading = %d, want 2", leading)
	}
	if trailing != 1 {
		t.Errorf("trailing = %d, want 1", trailing)
	}
}

func TestRemoveColumns(t *testing.T) {
	rows := CellGrid{
		{"A", "B", "C", "D"},
		{"1", "2", "3", "4"},
	}
	result := RemoveColumns(rows, []int{1, 3})
	if len(result) != 2 {
		t.Fatalf("expected 2 rows")
	}
	if len(result[0]) != 2 || result[0][0] != "A" || result[0][1] != "C" {
		t.Errorf("row 0 = %v, want [A C]", result[0])
	}
	if len(result[1]) != 2 || result[1][0] != "1" || result[1][1] != "3" {
		t.Errorf("row 1 = %v, want [1 3]", result[1])
	}
}

func TestDetectHeaderCandidates(t *testing.T) {
	rows := CellGrid{
		{"QUARTERLY SUMMARY LIST OF PURCHASES", "", "", ""}, // title
		{"", "", "", ""},                                     // blank
		{"TAXABLE MONTH", "TIN", "REGISTERED NAME", "AMOUNT OF GROSS PURCHASE"}, // header
		{"(1)", "(2)", "(3)", "(4)"},                        // numbering
		{"01-2024", "123-456-789-000", "Vendor A", "5000"},  // data
	}
	candidates := DetectHeaderCandidates(rows, 10)
	if len(candidates) == 0 {
		t.Fatal("expected at least one header candidate")
	}
	// Row 2 should be the top candidate
	if candidates[0] != 2 {
		t.Errorf("best header candidate = row %d, want 2", candidates[0])
	}
}

func TestDetectHeaderZone_MultiRow(t *testing.T) {
	rows := CellGrid{
		{"TAXPAYER", "", "", "AMOUNT OF", "AMOUNT OF"},
		{"IDENTIFICATION", "", "", "GROSS PURCHASE", "EXEMPT PURCHASE"},
		{"NUMBER", "", "", "", ""},
		{"(1)", "(2)", "(3)", "(4)", "(5)"},
		{"103-314-245", "Vendor", "Addr", "5000", "0"},
	}
	start, end, dataStart := DetectHeaderZone(rows, 1)
	if start != 0 {
		t.Errorf("zoneStart = %d, want 0", start)
	}
	if end != 2 {
		t.Errorf("zoneEnd = %d, want 2", end)
	}
	if dataStart != 4 {
		t.Errorf("dataStart = %d, want 4", dataStart)
	}
}

func TestMergeHeaderRows(t *testing.T) {
	rows := CellGrid{
		{"TAXPAYER", "", "AMOUNT OF"},
		{"IDENTIFICATION", "", "GROSS PURCHASE"},
		{"NUMBER", "", ""},
	}
	merged := MergeHeaderRows(rows, 0, 2)
	if len(merged) != 3 {
		t.Fatalf("merged len = %d, want 3", len(merged))
	}
	if merged[0] != "TAXPAYER IDENTIFICATION NUMBER" {
		t.Errorf("merged[0] = %q", merged[0])
	}
	if merged[1] != "" {
		t.Errorf("merged[1] = %q, want empty", merged[1])
	}
	if merged[2] != "AMOUNT OF GROSS PURCHASE" {
		t.Errorf("merged[2] = %q", merged[2])
	}
}

func TestCleanupColumns(t *testing.T) {
	cols := []string{"Name", "", "Amount", "", ""}
	cleaned := CleanupColumns(cols)
	if len(cleaned) != 3 {
		t.Fatalf("cleaned len = %d, want 3", len(cleaned))
	}
	if cleaned[0] != "Name" {
		t.Errorf("cleaned[0] = %q", cleaned[0])
	}
	if cleaned[1] != "Column_2" {
		t.Errorf("cleaned[1] = %q, want Column_2", cleaned[1])
	}
	if cleaned[2] != "Amount" {
		t.Errorf("cleaned[2] = %q", cleaned[2])
	}
}

func TestRunPhysicalHeuristics(t *testing.T) {
	rows := CellGrid{
		{"", "", ""},
		{"Name", "TIN", "Amount"},
		{"A", "123", "100"},
		{"B", "456", "200"},
		{"", "", ""},
	}
	result := RunPhysicalHeuristics(rows)
	if result.TrimmedTopRows != 1 {
		t.Errorf("TrimmedTopRows = %d, want 1", result.TrimmedTopRows)
	}
	if result.TrimmedBotRows != 1 {
		t.Errorf("TrimmedBotRows = %d, want 1", result.TrimmedBotRows)
	}
	if len(result.DataRegions) == 0 {
		t.Error("expected at least one data region")
	}
	if result.BestRegion == nil {
		t.Error("expected a best region")
	}
	if len(result.HeaderCandidates) == 0 {
		t.Error("expected at least one header candidate")
	}
}

func TestBestDataRegion_PreferLarger(t *testing.T) {
	regions := []DataRegion{
		{StartRow: 0, EndRow: 2, RowCount: 3},
		{StartRow: 5, EndRow: 15, RowCount: 11},
		{StartRow: 20, EndRow: 22, RowCount: 3},
	}
	best := BestDataRegion(regions)
	if best.StartRow != 5 {
		t.Errorf("best region starts at %d, want 5", best.StartRow)
	}
}

func TestBestDataRegion_Nil(t *testing.T) {
	best := BestDataRegion(nil)
	if best != nil {
		t.Error("expected nil for empty regions")
	}
}
