package service

import (
	"strings"
	"testing"
)

func TestParseUploadedFile_CSV(t *testing.T) {
	csv := "Name,Amount,Date\nAlice,1000,2024-01-15\nBob,2000,2024-02-20\n"
	content := []byte(csv)

	result, err := ParseUploadedFile(content, "test.csv")
	if err != nil {
		t.Fatalf("ParseUploadedFile(csv): %v", err)
	}

	if result.Type != "csv" {
		t.Errorf("Type = %q, want csv", result.Type)
	}

	sheet, ok := result.Sheets["Sheet1"]
	if !ok {
		t.Fatal("Expected Sheet1 in CSV result")
	}

	if len(sheet.Columns) != 3 {
		t.Errorf("Columns = %d, want 3", len(sheet.Columns))
	}
	if sheet.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", sheet.RowCount)
	}
	if len(sheet.Preview) != 2 {
		t.Errorf("Preview rows = %d, want 2", len(sheet.Preview))
	}
}

func TestParseUploadedFile_EmptyCSV(t *testing.T) {
	content := []byte("")

	_, err := ParseUploadedFile(content, "empty.csv")
	if err == nil {
		t.Error("Expected error for empty CSV")
	}
}

func TestParseUploadedFile_HeaderOnlyCSV(t *testing.T) {
	csv := "Col1,Col2,Col3\n"
	content := []byte(csv)

	result, err := ParseUploadedFile(content, "headers.csv")
	if err != nil {
		// Both behaviors are acceptable: error or empty result
		return
	}

	// If no error, the sheet should show 0 data rows
	sheet := result.Sheets["Sheet1"]
	if sheet != nil && sheet.RowCount != 0 {
		t.Errorf("Expected 0 rows for header-only CSV, got %d", sheet.RowCount)
	}
}

func TestParseUploadedFile_UnsupportedFormat(t *testing.T) {
	_, err := ParseUploadedFile([]byte("data"), "file.pdf")
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Expected 'unsupported' in error, got: %s", err.Error())
	}
}

func TestParseUploadedFile_CSVWithEmptyRows(t *testing.T) {
	csv := "A,B\n1,2\n,,\n3,4\n"
	content := []byte(csv)

	result, err := ParseUploadedFile(content, "gaps.csv")
	if err != nil {
		t.Fatalf("ParseUploadedFile: %v", err)
	}

	sheet := result.Sheets["Sheet1"]
	if sheet.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2 (empty rows should be skipped)", sheet.RowCount)
	}
}

func TestParseUploadedFile_CSVPreviewLimit(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("ID,Value\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("row,data\n")
	}
	content := []byte(sb.String())

	result, err := ParseUploadedFile(content, "many.csv")
	if err != nil {
		t.Fatalf("ParseUploadedFile: %v", err)
	}

	sheet := result.Sheets["Sheet1"]
	if sheet.RowCount != 50 {
		t.Errorf("RowCount = %d, want 50", sheet.RowCount)
	}
	if len(sheet.Preview) != MaxPreviewRows {
		t.Errorf("Preview = %d, want %d", len(sheet.Preview), MaxPreviewRows)
	}
}

func TestParseUploadedFile_InvalidExcel(t *testing.T) {
	// Random bytes are not a valid Excel file
	content := []byte("not an excel file at all")

	_, err := ParseUploadedFile(content, "bad.xlsx")
	if err == nil {
		t.Error("Expected error for invalid Excel content")
	}
}
