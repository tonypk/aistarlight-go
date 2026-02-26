package worker

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task type constants.
const (
	TypePDFGenerate      = "pdf:generate"
	TypeOCRProcess       = "ocr:process"
	TypeAIClassify       = "ai:classify"
	TypeBankReconcile    = "bank:reconcile"
	TypeCleanupOldTasks  = "cleanup:old_tasks"
	TypeComplianceCheck  = "compliance:validate"
	TypeDeadlineCheck    = "deadline:check"
)

// Queue names with priorities.
const (
	QueueCritical = "critical" // priority 6
	QueueDefault  = "default"  // priority 3
	QueueLow      = "low"      // priority 1
)

// PDFPayload is the payload for PDF generation tasks.
type PDFPayload struct {
	TaskID    uuid.UUID `json:"task_id"`
	CompanyID uuid.UUID `json:"company_id"`
	ReportID  uuid.UUID `json:"report_id"`
	Format    string    `json:"format"` // pdf, csv
}

// OCRPayload is the payload for OCR processing tasks.
type OCRPayload struct {
	TaskID     uuid.UUID `json:"task_id"`
	CompanyID  uuid.UUID `json:"company_id"`
	UserID     uuid.UUID `json:"user_id"`
	BatchID    uuid.UUID `json:"batch_id"`
	ImagePaths []string  `json:"image_paths"`
	Period     string    `json:"period"`
	ReportType string    `json:"report_type"`
}

// AIClassifyPayload is the payload for AI classification tasks.
type AIClassifyPayload struct {
	TaskID         uuid.UUID   `json:"task_id"`
	CompanyID      uuid.UUID   `json:"company_id"`
	SessionID      uuid.UUID   `json:"session_id"`
	TransactionIDs []uuid.UUID `json:"transaction_ids"`
}

// BankReconcilePayload is the payload for bank reconciliation tasks.
type BankReconcilePayload struct {
	TaskID          uuid.UUID              `json:"task_id"`
	CompanyID       uuid.UUID              `json:"company_id"`
	UserID          uuid.UUID              `json:"user_id"`
	Period          string                 `json:"period"`
	AmountTolerance float64                `json:"amount_tolerance"`
	DateTolerance   int                    `json:"date_tolerance_days"`
	Records         []map[string]string    `json:"records"`
	BankColumns     []string               `json:"bank_columns"`
	BankRows        []map[string]string    `json:"bank_rows"`
}

// CompliancePayload is the payload for compliance validation tasks.
type CompliancePayload struct {
	TaskID    uuid.UUID `json:"task_id"`
	CompanyID uuid.UUID `json:"company_id"`
	ReportID  uuid.UUID `json:"report_id"`
}

// NewPDFTask creates a new PDF generation task.
func NewPDFTask(p PDFPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypePDFGenerate, payload, asynq.Queue(QueueDefault), asynq.MaxRetry(3)), nil
}

// NewOCRTask creates a new OCR processing task.
func NewOCRTask(p OCRPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeOCRProcess, payload, asynq.Queue(QueueDefault), asynq.MaxRetry(3)), nil
}

// NewAIClassifyTask creates a new AI classification task.
func NewAIClassifyTask(p AIClassifyPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeAIClassify, payload, asynq.Queue(QueueDefault), asynq.MaxRetry(2)), nil
}

// NewBankReconcileTask creates a new bank reconciliation task.
func NewBankReconcileTask(p BankReconcilePayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeBankReconcile, payload, asynq.Queue(QueueDefault), asynq.MaxRetry(2)), nil
}

// NewComplianceTask creates a new compliance validation task.
func NewComplianceTask(p CompliancePayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeComplianceCheck, payload, asynq.Queue(QueueDefault), asynq.MaxRetry(2)), nil
}

// NewCleanupTask creates a periodic cleanup task.
func NewCleanupTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeCleanupOldTasks, nil, asynq.Queue(QueueLow), asynq.MaxRetry(1)), nil
}

// NewDeadlineCheckTask creates a periodic deadline check task.
func NewDeadlineCheckTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeDeadlineCheck, nil, asynq.Queue(QueueDefault), asynq.MaxRetry(2)), nil
}
