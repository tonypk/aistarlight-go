package integration

import "time"

// Event type constants from AIGoNHR.
const (
	EventPayrollRunCompleted = "payroll.run.completed"
	EventPayrollRunReversed  = "payroll.run.reversed"
	EventEmployeeUpserted    = "employee.upserted"
	EventEmployeeTerminated  = "employee.terminated"
)

// PayrollRunCompletedEvent is received from AIGoNHR when a payroll run is finalized.
// Used to create journal entries and fill tax forms.
type PayrollRunCompletedEvent struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	EventVersion int       `json:"event_version"`
	OccurredAt   time.Time `json:"occurred_at"`

	HRCompanyID  int64  `json:"hr_company_id"`
	Jurisdiction string `json:"jurisdiction"`
	Currency     string `json:"currency"`

	PayrollRunID int64  `json:"payroll_run_id"`
	CycleName    string `json:"cycle_name"`
	PayDate      string `json:"pay_date"`
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`

	Totals              PayrollTotals       `json:"totals"`
	DepartmentBreakdown []DeptSummary       `json:"department_breakdown"`
	EmployeeLines       []EmployeePayLine   `json:"employee_lines"`
	StatutoryPayables   []StatutoryPayable  `json:"statutory_payables"`
	WithholdingLines    []WithholdingLine   `json:"withholding_lines"`
}

// PayrollTotals summarizes the payroll run.
type PayrollTotals struct {
	GrossPay           string `json:"gross_pay"`
	TotalDeductions    string `json:"total_deductions"`
	TotalContributions string `json:"total_contributions"`
	NetPay             string `json:"net_pay"`
	HeadCount          int    `json:"head_count"`
}

// DeptSummary breaks down totals by department.
type DeptSummary struct {
	DepartmentID   int64  `json:"department_id"`
	DepartmentName string `json:"department_name"`
	GrossPay       string `json:"gross_pay"`
	NetPay         string `json:"net_pay"`
	HeadCount      int    `json:"head_count"`
}

// EmployeePayLine is one employee's payslip summary.
type EmployeePayLine struct {
	EmployeeID   int64            `json:"employee_id"`
	EmployeeNo   string           `json:"employee_no"`
	FullName     string           `json:"full_name"`
	TIN          string           `json:"tin,omitempty"`
	DepartmentID int64            `json:"department_id"`
	Earnings     []EarningLine    `json:"earnings"`
	Deductions   []DeductionLine  `json:"deductions"`
	NetPay       string           `json:"net_pay"`
}

// EarningLine is a single earning component.
type EarningLine struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
}

// DeductionLine is a single deduction component.
type DeductionLine struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
}

// StatutoryPayable represents employer+employee contributions to a statutory body.
type StatutoryPayable struct {
	Code             string `json:"code"`
	Label            string `json:"label"`
	EmployeeAmount   string `json:"employee_amount"`
	EmployerAmount   string `json:"employer_amount"`
	TotalAmount      string `json:"total_amount"`
}

// WithholdingLine represents tax withheld per employee.
type WithholdingLine struct {
	EmployeeID int64  `json:"employee_id"`
	EmployeeNo string `json:"employee_no"`
	FullName   string `json:"full_name"`
	TIN        string `json:"tin,omitempty"`
	TaxAmount  string `json:"tax_amount"`
}

// PayrollRunReversedEvent is received when a payroll run is reversed.
type PayrollRunReversedEvent struct {
	EventID           string    `json:"event_id"`
	EventType         string    `json:"event_type"`
	EventVersion      int       `json:"event_version"`
	OccurredAt        time.Time `json:"occurred_at"`
	HRCompanyID       int64     `json:"hr_company_id"`
	PayrollRunID      int64     `json:"payroll_run_id"`
	OriginalPayDate   string    `json:"original_pay_date"`
	ReversalReason    string    `json:"reversal_reason"`
}

// EmployeeUpsertedEvent is received when an employee is created or updated.
type EmployeeUpsertedEvent struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	EventVersion int       `json:"event_version"`
	OccurredAt   time.Time `json:"occurred_at"`

	HRCompanyID  int64  `json:"hr_company_id"`
	Jurisdiction string `json:"jurisdiction"`

	EmployeeID     int64  `json:"employee_id"`
	EmployeeNo     string `json:"employee_no"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Email          string `json:"email"`
	TIN            string `json:"tin,omitempty"`
	SSS            string `json:"sss,omitempty"`
	PhilHealth     string `json:"philhealth,omitempty"`
	PagIBIG        string `json:"pagibig,omitempty"`
	DepartmentID   int64  `json:"department_id"`
	DepartmentName string `json:"department_name"`
	PositionTitle  string `json:"position_title"`
	Status         string `json:"status"`
}

// EmployeeTerminatedEvent is received when an employee is terminated.
type EmployeeTerminatedEvent struct {
	EventID        string    `json:"event_id"`
	EventType      string    `json:"event_type"`
	EventVersion   int       `json:"event_version"`
	OccurredAt     time.Time `json:"occurred_at"`
	HRCompanyID    int64     `json:"hr_company_id"`
	EmployeeID     int64     `json:"employee_id"`
	EmployeeNo     string    `json:"employee_no"`
	TerminationDate string   `json:"termination_date"`
	Reason         string    `json:"reason"`
}

// WebhookPayload wraps any event for webhook delivery.
type WebhookPayload struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	Data      any       `json:"data"`
	Signature string    `json:"signature,omitempty"`
}
