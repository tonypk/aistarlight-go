package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// NotificationService manages filing deadline reminders and notifications.
type NotificationService struct {
	q *sqlc.Queries
}

// NewNotificationService creates a NotificationService.
func NewNotificationService(q *sqlc.Queries) *NotificationService {
	return &NotificationService{q: q}
}

// NotificationResponse is the API response for a notification.
type NotificationResponse struct {
	ID               string      `json:"id"`
	NotificationType string      `json:"notification_type"`
	Title            string      `json:"title"`
	Message          string      `json:"message"`
	Metadata         interface{} `json:"metadata"`
	IsRead           bool        `json:"is_read"`
	CreatedAt        string      `json:"created_at"`
	ReadAt           *string     `json:"read_at,omitempty"`
}

// ListForCompany returns paginated notifications.
func (s *NotificationService) ListForCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]NotificationResponse, int, error) {
	rows, err := s.q.ListNotificationsByCompany(ctx, sqlc.ListNotificationsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}

	total, _ := s.q.CountNotificationsByCompany(ctx, companyID)

	result := make([]NotificationResponse, len(rows))
	for i, n := range rows {
		result[i] = notificationToResponse(n)
	}
	return result, int(total), nil
}

// CountUnread returns the number of unread notifications.
func (s *NotificationService) CountUnread(ctx context.Context, companyID uuid.UUID) (int64, error) {
	return s.q.CountUnreadNotifications(ctx, companyID)
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(ctx context.Context, notificationID, companyID uuid.UUID) error {
	return s.q.MarkNotificationRead(ctx, sqlc.MarkNotificationReadParams{
		ID:        notificationID,
		CompanyID: companyID,
	})
}

// MarkAllRead marks all notifications for a company as read.
func (s *NotificationService) MarkAllRead(ctx context.Context, companyID uuid.UUID) error {
	return s.q.MarkAllNotificationsRead(ctx, companyID)
}

// CheckDeadlinesAndNotify checks all companies for upcoming filing deadlines and creates notifications.
func (s *NotificationService) CheckDeadlinesAndNotify(ctx context.Context) error {
	companies, err := s.q.ListAllCompanies(ctx)
	if err != nil {
		return fmt.Errorf("list companies: %w", err)
	}

	now := time.Now()
	formTypes := []struct {
		Form     string
		Name     string
		Deadline int // day of following month
	}{
		{birforms.FormBIR2550M, "Monthly VAT (2550M)", 20},
		{birforms.FormBIR1601C, "Withholding Tax on Compensation (1601C)", 10},
		{birforms.FormBIR0619E, "Expanded Withholding Tax (0619E)", 10},
	}

	for _, c := range companies {
		for _, ft := range formTypes {
			// Current period = last month
			period := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.Local)
			periodStr := period.Format("2006-01")

			// Deadline is in the current month
			deadline := time.Date(now.Year(), now.Month(), ft.Deadline, 0, 0, 0, 0, time.Local)
			daysUntil := int(deadline.Sub(now).Hours() / 24)

			var notifType, title, msg string

			switch {
			case daysUntil < 0:
				notifType = "deadline_overdue"
				title = fmt.Sprintf("OVERDUE: %s for %s", ft.Name, periodStr)
				msg = fmt.Sprintf("%s for period %s was due %s. File immediately to minimize penalties (25%% surcharge + 20%%/year interest).",
					ft.Name, periodStr, deadline.Format("January 2"))
			case daysUntil <= 3:
				notifType = "deadline_3day"
				title = fmt.Sprintf("URGENT: %s due in %d days", ft.Name, daysUntil)
				msg = fmt.Sprintf("%s for period %s is due %s (%d day(s) remaining).",
					ft.Name, periodStr, deadline.Format("January 2"), daysUntil)
			case daysUntil <= 7:
				notifType = "deadline_7day"
				title = fmt.Sprintf("%s due in %d days", ft.Name, daysUntil)
				msg = fmt.Sprintf("%s for period %s is due %s.",
					ft.Name, periodStr, deadline.Format("January 2"))
			default:
				continue // no notification needed
			}

			dedupKey := fmt.Sprintf("%s:%s:%s", notifType, ft.Form, periodStr)
			metadata, _ := json.Marshal(map[string]string{
				"form_type": ft.Form,
				"period":    periodStr,
				"deadline":  deadline.Format("2006-01-02"),
			})

			_, err := s.q.CreateNotification(ctx, sqlc.CreateNotificationParams{
				ID:               uuid.New(),
				CompanyID:        c.ID,
				UserID:           pgtype.UUID{}, // broadcast to all company members
				NotificationType: notifType,
				Title:            title,
				Message:          msg,
				Metadata:         metadata,
				DedupKey:         &dedupKey,
			})
			if err != nil {
				slog.Warn("create notification failed (likely dedup)", "company", c.ID, "dedup_key", dedupKey, "error", err)
			}
		}
	}

	return nil
}

func notificationToResponse(n sqlc.Notification) NotificationResponse {
	resp := NotificationResponse{
		ID:               n.ID.String(),
		NotificationType: n.NotificationType,
		Title:            n.Title,
		Message:          n.Message,
		IsRead:           n.IsRead,
		CreatedAt:        n.CreatedAt.Format(time.RFC3339),
	}
	if len(n.Metadata) > 0 {
		var m interface{}
		_ = json.Unmarshal(n.Metadata, &m)
		resp.Metadata = m
	}
	if n.ReadAt.Valid {
		t := n.ReadAt.Time.Format(time.RFC3339)
		resp.ReadAt = &t
	}
	return resp
}
