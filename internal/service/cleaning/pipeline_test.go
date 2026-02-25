package cleaning

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPipeline_HeuristicOnly(t *testing.T) {
	// Test the pipeline with no AI and no template
	rows := CellGrid{
		{"", "", "", "", ""},                                                      // 0: blank
		{"QUARTERLY SUMMARY LIST OF PURCHASES", "", "", "", ""},                   // 1: title
		{"TAXABLE MONTH", "TAXPAYER IDENTIFICATION NUMBER", "REGISTERED NAME", "AMOUNT OF GROSS PURCHASE", "AMOUNT OF INPUT TAX"}, // 2: header
		{"(1)", "(2)", "(3)", "(4)", "(5)"},                                       // 3: numbering
		{"01/2024", "123-456-789-000", "Vendor A Corp.", "5000.00", "600.00"},     // 4: data
		{"01/2024", "234-567-890-000", "Vendor B Inc.", "3000.00", "360.00"},      // 5: data
		{"01/2024", "345-678-901-000", "Total Logistics Inc.", "2000.00", "240.00"}, // 6: data (false positive protection)
		{"", "", "", "", ""},                                                      // 7: blank
		{"", "Sub Total", "", "10000.00", "1200.00"},                              // 8: subtotal
		{"", "Grand Total", "", "10000.00", "1200.00"},                            // 9: grand total
		{"", "Prepared by: Accountant", "", "", ""},                               // 10: note
	}

	pipeline := NewPipeline(nil, nil)
	result, err := pipeline.Run(context.Background(), rows, PipelineConfig{
		CompanyID: uuid.New(),
		SkipAI:    true,
	})
	if err != nil {
		t.Fatalf("Pipeline.Run: %v", err)
	}

	// Should have 3 data rows (vendors A, B, and Total Logistics)
	if len(result.DataRows) < 2 {
		t.Errorf("expected at least 2 data rows, got %d", len(result.DataRows))
	}

	// Report should show dropped rows
	if result.Report.OriginalRows != 11 {
		t.Errorf("OriginalRows = %d, want 11", result.Report.OriginalRows)
	}
	if len(result.Report.DroppedRows) == 0 {
		t.Error("expected some dropped rows in report")
	}

	// Check that columns were detected
	if len(result.Columns) < 3 {
		t.Errorf("expected at least 3 columns, got %d: %v", len(result.Columns), result.Columns)
	}

	// Overall confidence should be > 0
	if result.Report.OverallConfidence <= 0 {
		t.Errorf("OverallConfidence = %f, should be > 0", result.Report.OverallConfidence)
	}
}

func TestPipeline_EmptyInput(t *testing.T) {
	pipeline := NewPipeline(nil, nil)
	_, err := pipeline.Run(context.Background(), nil, PipelineConfig{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestPipeline_SimpleCSV(t *testing.T) {
	rows := CellGrid{
		{"Name", "Amount", "Date"},
		{"Alice", "1000", "01/15/2024"},
		{"Bob", "2000", "01/20/2024"},
		{"Charlie", "3000", "01/25/2024"},
	}

	pipeline := NewPipeline(nil, nil)
	result, err := pipeline.Run(context.Background(), rows, PipelineConfig{
		CompanyID: uuid.New(),
		SkipAI:    true,
	})
	if err != nil {
		t.Fatalf("Pipeline.Run: %v", err)
	}

	if len(result.DataRows) != 3 {
		t.Errorf("expected 3 data rows, got %d", len(result.DataRows))
	}
	if len(result.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(result.Columns))
	}
}

func TestPipeline_WithDroppedColumns(t *testing.T) {
	rows := CellGrid{
		{"Name", "", "Amount", "", "Date"},
		{"Alice", "", "1000", "", "01/15/2024"},
		{"Bob", "", "2000", "", "01/20/2024"},
	}

	pipeline := NewPipeline(nil, nil)
	result, err := pipeline.Run(context.Background(), rows, PipelineConfig{
		CompanyID: uuid.New(),
		SkipAI:    true,
	})
	if err != nil {
		t.Fatalf("Pipeline.Run: %v", err)
	}

	// Should detect 2 empty columns
	if len(result.Report.DroppedColumns) < 2 {
		t.Errorf("expected at least 2 dropped columns, got %d", len(result.Report.DroppedColumns))
	}
}

func TestBuildReport(t *testing.T) {
	classifications := []RowClassification{
		{RowIndex: 0, Type: RowTypeData, Confidence: 0.9},
		{RowIndex: 1, Type: RowTypeData, Confidence: 0.95},
		{RowIndex: 2, Type: RowTypeSubtotal, Confidence: 0.8},
		{RowIndex: 3, Type: RowTypeGrandTotal, Confidence: 0.95},
	}

	report := BuildReport(10, 5, 2,
		[]DroppedItem{{Index: 2, Reason: "subtotal"}},
		nil,
		nil,
		classifications,
		false,
		"purchase",
	)

	if report.TableType != "purchase" {
		t.Errorf("TableType = %q", report.TableType)
	}
	if report.OriginalRows != 10 {
		t.Errorf("OriginalRows = %d", report.OriginalRows)
	}
	if report.RetainedDataRows != 2 {
		t.Errorf("RetainedDataRows = %d", report.RetainedDataRows)
	}
	if report.OverallConfidence <= 0 || report.OverallConfidence > 1.0 {
		t.Errorf("OverallConfidence = %f", report.OverallConfidence)
	}
}
