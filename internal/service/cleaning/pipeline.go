package cleaning

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// PipelineConfig holds parameters for the cleaning pipeline.
type PipelineConfig struct {
	// CompanyID is required for template matching/saving.
	CompanyID uuid.UUID

	// Period for date validation (e.g., "2024-01").
	Period string

	// TableTypeHint overrides AI table type detection if set.
	TableTypeHint string

	// SkipAI disables AI analysis (uses heuristics only).
	SkipAI bool

	// SkipTemplateMatch disables template matching.
	SkipTemplateMatch bool

	// ClassifierConfig overrides for row classifier.
	ClassifierConfig *ClassifierConfig

	// ValidatorConfig overrides for validator.
	ValidatorConfig *ValidatorConfig
}

// Pipeline orchestrates the multi-stage cleaning process.
type Pipeline struct {
	aiSvc       *AISemanticService
	templateSvc *TemplateService
}

// NewPipeline creates a cleaning pipeline.
func NewPipeline(aiSvc *AISemanticService, templateSvc *TemplateService) *Pipeline {
	return &Pipeline{
		aiSvc:       aiSvc,
		templateSvc: templateSvc,
	}
}

// Run executes the full cleaning pipeline on raw spreadsheet rows.
func (p *Pipeline) Run(ctx context.Context, rows CellGrid, cfg PipelineConfig) (*CleaningResult, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("no rows to process")
	}

	originalRows := len(rows)
	originalCols := MaxColWidth(rows)

	// Stage 1: Physical heuristics
	slog.Info("cleaning pipeline: stage 1 - physical heuristics", "rows", originalRows)
	physical := RunPhysicalHeuristics(rows)

	// Build dropped columns from physical pass
	var droppedCols []DroppedItem
	for _, col := range physical.PrunedColumns {
		droppedCols = append(droppedCols, DroppedItem{
			Index:      col,
			Reason:     "empty_column",
			Preview:    "Column " + itoa(col+1),
			Confidence: 1.0,
		})
	}

	// Stage 2: Template match
	var aiResult *AISemanticResult
	templateMatched := false

	if !cfg.SkipTemplateMatch && p.templateSvc != nil && physical.BestRegion != nil {
		slog.Info("cleaning pipeline: stage 2 - template match")
		// Use header candidates to extract column names for matching
		if len(physical.HeaderCandidates) > 0 {
			headerIdx := physical.HeaderCandidates[0]
			zoneStart, zoneEnd, _ := DetectHeaderZone(rows, headerIdx)
			columns := MergeHeaderRows(rows, zoneStart, zoneEnd)
			columns = CleanupColumns(columns)

			matched, err := p.templateSvc.MatchTemplate(ctx, cfg.CompanyID, columns)
			if err != nil {
				slog.Warn("template match error", "error", err)
			} else if matched != nil {
				aiResult = matched
				templateMatched = true
				slog.Info("cleaning pipeline: template matched, skipping AI",
					"table_type", aiResult.TableType,
				)
			}
		}
	}

	// Stage 3: AI semantic pass (if no template match)
	if aiResult == nil && !cfg.SkipAI && p.aiSvc != nil {
		slog.Info("cleaning pipeline: stage 3 - AI semantic analysis")
		result, err := p.aiSvc.Analyze(ctx, rows)
		if err != nil {
			slog.Warn("AI semantic analysis failed, falling back to heuristics", "error", err)
		} else {
			aiResult = result
		}
	}

	// Resolve columns and data boundaries
	var columns []string
	var dataStartRow, dataEndRow int
	var columnMapping map[string]FieldMapping

	if aiResult != nil {
		columns = aiResult.Columns
		dataStartRow = aiResult.DataStartRow
		dataEndRow = aiResult.DataEndRow
		columnMapping = aiResult.ColumnMapping

		// Merge AI drop columns with physical pruned columns
		for _, col := range aiResult.DropColumns {
			alreadyDropped := false
			for _, dc := range droppedCols {
				if dc.Index == col {
					alreadyDropped = true
					break
				}
			}
			if !alreadyDropped {
				preview := "Column " + itoa(col+1)
				if col < len(columns) {
					preview = columns[col]
				}
				droppedCols = append(droppedCols, DroppedItem{
					Index:      col,
					Reason:     "ai_drop_rule",
					Preview:    preview,
					Confidence: 0.9,
				})
			}
		}
	} else {
		// Fallback: heuristic-only
		if len(physical.HeaderCandidates) > 0 {
			headerIdx := physical.HeaderCandidates[0]
			zoneStart, zoneEnd, ds := DetectHeaderZone(rows, headerIdx)
			if zoneStart == zoneEnd {
				for _, h := range rows[headerIdx] {
					columns = append(columns, strings.TrimSpace(h))
				}
			} else {
				columns = MergeHeaderRows(rows, zoneStart, zoneEnd)
			}
			columns = CleanupColumns(columns)
			dataStartRow = ds
		} else {
			// No header detected, use row 0 as header
			for _, h := range rows[0] {
				columns = append(columns, strings.TrimSpace(h))
			}
			columns = CleanupColumns(columns)
			dataStartRow = 1
		}
		dataEndRow = len(rows) - 1
		columnMapping = make(map[string]FieldMapping)
	}

	// Determine table type
	tableType := "unknown"
	if aiResult != nil {
		tableType = aiResult.TableType
	}
	if cfg.TableTypeHint != "" {
		tableType = cfg.TableTypeHint
	}

	// Stage 4: Row classification
	slog.Info("cleaning pipeline: stage 4 - row classification",
		"data_range", fmt.Sprintf("[%d, %d]", dataStartRow, dataEndRow),
	)
	classifierCfg := DefaultClassifierConfig()
	if cfg.ClassifierConfig != nil {
		classifierCfg = *cfg.ClassifierConfig
	}

	totalCols := len(columns)
	if totalCols == 0 {
		totalCols = MaxColWidth(rows)
	}
	classifier := NewRowClassifier(columns, totalCols, classifierCfg)
	classifications := classifier.ClassifyRows(rows, dataStartRow, dataEndRow)

	// Build data rows and dropped rows
	var dataRows []map[string]interface{}
	var rawDataRows CellGrid
	var droppedRows []DroppedItem

	for _, cls := range classifications {
		if cls.RowIndex >= len(rows) {
			continue
		}
		row := rows[cls.RowIndex]

		switch cls.Type {
		case RowTypeData:
			m := make(map[string]interface{}, len(columns))
			for i, col := range columns {
				if i < len(row) {
					val := strings.TrimSpace(row[i])
					if val == "" {
						m[col] = nil
					} else {
						m[col] = val
					}
				} else {
					m[col] = nil
				}
			}
			dataRows = append(dataRows, m)
			rawDataRows = append(rawDataRows, row)

		case RowTypeBlank:
			droppedRows = append(droppedRows, DroppedItem{
				Index:      cls.RowIndex,
				Reason:     "blank",
				Preview:    "",
				Confidence: cls.Confidence,
			})

		default:
			droppedRows = append(droppedRows, DroppedItem{
				Index:      cls.RowIndex,
				Reason:     string(cls.Type),
				Preview:    RowPreview(row),
				Confidence: cls.Confidence,
			})
		}
	}

	// Stage 5: Validation
	slog.Info("cleaning pipeline: stage 5 - validation",
		"data_rows", len(dataRows),
	)
	validatorCfg := DefaultValidatorConfig()
	if cfg.ValidatorConfig != nil {
		validatorCfg = *cfg.ValidatorConfig
	}
	validatorCfg.Period = cfg.Period
	validatorCfg.TableType = tableType

	var validationIssues []ValidationIssue
	if len(dataRows) > 0 && len(columnMapping) > 0 {
		validationIssues = Validate(dataRows, columns, columnMapping, validatorCfg)
	}

	// Stage 6: Build report
	slog.Info("cleaning pipeline: stage 6 - report generation")
	report := BuildReport(
		originalRows, originalCols,
		len(dataRows),
		droppedRows, droppedCols,
		validationIssues,
		classifications,
		templateMatched,
		tableType,
	)

	// Stage 7: Save template (if AI was used and successful)
	if !templateMatched && aiResult != nil && p.templateSvc != nil {
		slog.Info("cleaning pipeline: stage 7 - saving template")
		if err := p.templateSvc.SaveTemplate(ctx, cfg.CompanyID, columns, aiResult); err != nil {
			slog.Warn("failed to save cleaning template", "error", err)
		}
	}

	return &CleaningResult{
		Columns:       columns,
		ColumnMapping: columnMapping,
		DataRows:      dataRows,
		RawDataRows:   rawDataRows,
		Report:        report,
		AIResult:      aiResult,
	}, nil
}
