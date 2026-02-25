package cleaning

// BuildReport generates a CleaningReport from the pipeline results.
func BuildReport(
	originalRows int,
	originalCols int,
	retainedRows int,
	droppedRows []DroppedItem,
	droppedCols []DroppedItem,
	validationIssues []ValidationIssue,
	classifications []RowClassification,
	templateMatched bool,
	tableType string,
) CleaningReport {
	report := CleaningReport{
		TableType:        tableType,
		OriginalRows:     originalRows,
		OriginalCols:     originalCols,
		RetainedDataRows: retainedRows,
		DroppedRows:      droppedRows,
		DroppedColumns:   droppedCols,
		ValidationIssues: validationIssues,
		TemplateMatched:  templateMatched,
	}

	if report.DroppedRows == nil {
		report.DroppedRows = []DroppedItem{}
	}
	if report.DroppedColumns == nil {
		report.DroppedColumns = []DroppedItem{}
	}
	if report.ValidationIssues == nil {
		report.ValidationIssues = []ValidationIssue{}
	}

	// Build review items from low-confidence classifications
	report.ReviewItems = buildReviewItems(classifications, validationIssues)
	if report.ReviewItems == nil {
		report.ReviewItems = []ReviewItem{}
	}

	// Calculate overall confidence
	report.OverallConfidence = calculateOverallConfidence(
		retainedRows, originalRows,
		len(validationIssues),
		classifications,
		templateMatched,
	)

	return report
}

// buildReviewItems identifies items that need human review.
func buildReviewItems(classifications []RowClassification, issues []ValidationIssue) []ReviewItem {
	var items []ReviewItem

	// Flag rows with borderline classification confidence
	for _, cls := range classifications {
		if cls.Type != RowTypeData && cls.Confidence < 0.7 && cls.Confidence > 0.3 {
			items = append(items, ReviewItem{
				RowIndex:    cls.RowIndex,
				Description: "borderline classification as " + string(cls.Type) + ": " + cls.Reason,
				Confidence:  cls.Confidence,
			})
		}
	}

	// Flag error-level validation issues
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			items = append(items, ReviewItem{
				RowIndex:    iss.RowIndex,
				Description: iss.Field + ": " + iss.Message,
				Confidence:  0.0,
			})
		}
	}

	return items
}

// calculateOverallConfidence produces a 0–1 score reflecting data quality.
func calculateOverallConfidence(
	retainedRows, originalRows int,
	issueCount int,
	classifications []RowClassification,
	templateMatched bool,
) float64 {
	if originalRows == 0 {
		return 0
	}

	// Base: retained/original ratio
	retentionRatio := float64(retainedRows) / float64(originalRows)

	// Penalty for validation issues (each issue reduces confidence slightly)
	issuePenalty := float64(issueCount) * 0.01
	if issuePenalty > 0.3 {
		issuePenalty = 0.3
	}

	// Bonus for template match (well-known format)
	templateBonus := 0.0
	if templateMatched {
		templateBonus = 0.1
	}

	// Average classification confidence for non-data rows
	classConfidence := 0.0
	classCount := 0
	for _, cls := range classifications {
		if cls.Type != RowTypeData && cls.Type != RowTypeBlank {
			classConfidence += cls.Confidence
			classCount++
		}
	}
	if classCount > 0 {
		classConfidence /= float64(classCount)
	} else {
		classConfidence = 1.0 // no non-data rows to classify, no uncertainty
	}

	overall := 0.4*retentionRatio + 0.3*classConfidence + 0.3*(1.0-issuePenalty) + templateBonus
	if overall > 1.0 {
		overall = 1.0
	}
	if overall < 0.0 {
		overall = 0.0
	}

	return overall
}
