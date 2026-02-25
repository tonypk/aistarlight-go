package cleaning

// RowType classifies each row in the spreadsheet.
type RowType string

const (
	RowTypeData         RowType = "data"
	RowTypeHeader       RowType = "header"
	RowTypeHeaderRepeat RowType = "header_repeat"
	RowTypeSubtotal     RowType = "subtotal"
	RowTypeGrandTotal   RowType = "grand_total"
	RowTypeNote         RowType = "note"
	RowTypeBlank        RowType = "blank"
	RowTypeNumbering    RowType = "numbering"
)

// CellGrid is the raw 2D string matrix from a spreadsheet.
type CellGrid = [][]string

// RowClassification holds the classification result for a single row.
type RowClassification struct {
	RowIndex   int     `json:"row_index"`
	Type       RowType `json:"type"`
	Confidence float64 `json:"confidence"` // 0.0–1.0
	Reason     string  `json:"reason"`
}

// DataRegion represents a contiguous block of data rows.
type DataRegion struct {
	StartRow int `json:"start_row"` // inclusive
	EndRow   int `json:"end_row"`   // inclusive
	RowCount int `json:"row_count"` // number of non-empty rows in the region
}

// FieldMapping maps a source column to a target BIR field.
type FieldMapping struct {
	TargetField string  `json:"target_field"`
	Confidence  float64 `json:"confidence"`
}

// AISemanticResult is the merged output from a single AI call that combines
// header detection, column mapping, and drop rules.
type AISemanticResult struct {
	TableType     string                  `json:"table_type"`      // sales|purchase|bank|payroll|ewt|itr|unknown
	HeaderRows    []int                   `json:"header_rows"`     // 0-indexed row indices that form the header
	DataStartRow  int                     `json:"data_start_row"`  // 0-indexed
	DataEndRow    int                     `json:"data_end_row"`    // 0-indexed, inclusive
	Columns       []string                `json:"columns"`         // merged column names
	DropColumns   []int                   `json:"drop_columns"`    // 0-indexed column indices to remove
	ColumnMapping map[string]FieldMapping `json:"column_mapping"`  // source col name → target field
	Warnings      []string               `json:"warnings"`
}

// DroppedItem records why a row or column was dropped.
type DroppedItem struct {
	Index      int     `json:"index"`      // row or column index (0-based in original)
	Reason     string  `json:"reason"`     // e.g. "summary_row", "blank", "header_repeat"
	Preview    string  `json:"preview"`    // first ~100 chars of the row/column for audit
	Confidence float64 `json:"confidence"` // how confident we are this should be dropped
}

// ValidationSeverity indicates the severity of a validation issue.
type ValidationSeverity string

const (
	SeverityError   ValidationSeverity = "error"
	SeverityWarning ValidationSeverity = "warning"
	SeverityInfo    ValidationSeverity = "info"
)

// ValidationIssue records a data quality problem found during validation.
type ValidationIssue struct {
	RowIndex int                `json:"row_index"` // -1 for sheet-level issues
	Column   string             `json:"column"`
	Field    string             `json:"field"` // target field name
	Value    string             `json:"value"` // the problematic value
	Message  string             `json:"message"`
	Severity ValidationSeverity `json:"severity"`
}

// ReviewItem flags something that needs human review.
type ReviewItem struct {
	RowIndex    int     `json:"row_index"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"` // how confident we are it's correct as-is
}

// CleaningReport is the audit trail of all cleaning decisions.
type CleaningReport struct {
	TableType        string            `json:"table_type"`
	OriginalRows     int               `json:"original_rows"`
	OriginalCols     int               `json:"original_cols"`
	RetainedDataRows int               `json:"retained_data_rows"`
	DroppedRows      []DroppedItem     `json:"dropped_rows"`
	DroppedColumns   []DroppedItem     `json:"dropped_columns"`
	ValidationIssues []ValidationIssue `json:"validation_issues"`
	OverallConfidence float64          `json:"overall_confidence"` // 0.0–1.0
	ReviewItems      []ReviewItem      `json:"review_items"`
	TemplateMatched  bool              `json:"template_matched"`
	TemplateName     string            `json:"template_name,omitempty"`
}

// CleaningResult is the final output of the cleaning pipeline.
type CleaningResult struct {
	Columns       []string                 `json:"columns"`        // cleaned column names
	ColumnMapping map[string]FieldMapping  `json:"column_mapping"` // source → target field
	DataRows      []map[string]interface{} `json:"data_rows"`      // cleaned data rows
	RawDataRows   CellGrid                 `json:"raw_data_rows"`  // raw string rows (for debugging)
	Report        CleaningReport           `json:"report"`
	AIResult      *AISemanticResult        `json:"ai_result,omitempty"`
}

// PhysicalResult holds the output of physical heuristic analysis.
type PhysicalResult struct {
	DataRegions      []DataRegion `json:"data_regions"`
	BestRegion       *DataRegion  `json:"best_region"`
	HeaderCandidates []int        `json:"header_candidates"` // row indices scored as potential headers
	PrunedColumns    []int        `json:"pruned_columns"`    // indices of empty columns removed
	TrimmedTopRows   int          `json:"trimmed_top_rows"`  // count of leading blank rows
	TrimmedBotRows   int          `json:"trimmed_bot_rows"`  // count of trailing blank rows
}
