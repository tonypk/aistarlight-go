package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

func (s *Server) handlePDFGenerate(ctx context.Context, t *asynq.Task) error {
	var p PDFPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal pdf payload: %w", err)
	}

	slog.Info("pdf task started", "task_id", p.TaskID, "report_id", p.ReportID)

	_ = s.q.UpdateAsyncTaskStatus(ctx, sqlc.UpdateAsyncTaskStatusParams{
		ID: p.TaskID, Status: "processing",
	})
	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 10,
	})

	// Load report data.
	report, err := s.q.GetReportByID(ctx, p.ReportID)
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("get report: %w", err))
	}

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 30,
	})

	// Parse calculated data.
	var calcData map[string]string
	if err := json.Unmarshal(report.CalculatedData, &calcData); err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("unmarshal calculated data: %w", err))
	}
	calcData["period"] = report.Period

	// Load company info.
	company, err := s.q.GetCompanyByID(ctx, report.CompanyID)
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("get company: %w", err))
	}

	companyInfo := service.CompanyInfo{
		CompanyName: company.CompanyName,
	}
	if company.TinNumber != nil {
		companyInfo.TINNumber = *company.TinNumber
	}
	if company.RdoCode != nil {
		companyInfo.RDOCode = *company.RdoCode
	}

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 50,
	})

	// Generate PDF.
	var buf bytes.Buffer
	if err := service.GeneratePDFReport(&buf, report.ReportType, calcData, companyInfo); err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("generate pdf: %w", err))
	}

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 80,
	})

	// Save PDF file.
	tin := "no_tin"
	if company.TinNumber != nil {
		tin = *company.TinNumber
	}
	filename := fmt.Sprintf("%s_%s_%s.pdf", report.ReportType, report.Period, tin)
	pdfDir := filepath.Join(s.uploadDir, "reports", report.CompanyID.String())
	if err := os.MkdirAll(pdfDir, 0o755); err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("create pdf dir: %w", err))
	}
	pdfPath := filepath.Join(pdfDir, filename)
	if err := os.WriteFile(pdfPath, buf.Bytes(), 0o644); err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("write pdf: %w", err))
	}

	// Update report with file path (using existing version for optimistic lock).
	_ = s.q.UpdateReport(ctx, sqlc.UpdateReportParams{
		ID:       p.ReportID,
		Status:   report.Status,
		FilePath: &pdfPath,
		Version:  report.Version,
	})

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 100,
	})

	result, _ := json.Marshal(map[string]interface{}{
		"report_id":   p.ReportID.String(),
		"report_type": report.ReportType,
		"period":      report.Period,
		"file_path":   pdfPath,
		"file_size":   buf.Len(),
		"status":      "completed",
	})
	if err := s.q.UpdateAsyncTaskResult(ctx, sqlc.UpdateAsyncTaskResultParams{
		ID: p.TaskID, Result: result,
	}); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	slog.Info("pdf generated",
		"task_id", p.TaskID,
		"report_id", p.ReportID,
		"path", pdfPath,
		"size", buf.Len(),
	)
	return nil
}

// GeneratePDFFile is a convenience function for synchronous PDF generation.
func GeneratePDFFile(reportType string, data map[string]string, company service.CompanyInfo, outputPath string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	var buf bytes.Buffer
	if err := service.GeneratePDFReport(&buf, reportType, data, company); err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

