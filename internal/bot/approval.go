package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/service"
)

// Inline keyboard buttons for receipt approval.
var (
	btnApprove = tele.Btn{Unique: "rcpt_ap", Text: "Approve"}
	btnReject  = tele.Btn{Unique: "rcpt_rj", Text: "Reject"}
)

// checkAndRequestApproval evaluates whether a receipt batch needs approval.
// Returns true if approval is required (processing should be paused).
func (b *Bot) checkAndRequestApproval(c tele.Context, ctx context.Context, batchID, companyID, userID uuid.UUID, amount float64, vendorName string, isDuplicate bool) bool {
	if b.approvals == nil {
		return false
	}

	result, err := b.approvals.EvaluateApproval(ctx, companyID, amount, vendorName, isDuplicate)
	if err != nil {
		slog.Warn("approval check failed, proceeding without approval", "error", err)
		return false
	}

	if !result.NeedsApproval {
		return false
	}

	// Create the approval record.
	approval, err := b.approvals.CreateApproval(ctx, batchID, companyID, userID, result.TriggerReason)
	if err != nil {
		slog.Warn("failed to create approval, proceeding without", "error", err)
		return false
	}

	// Notify the user that approval is required.
	reasonText := ""
	switch result.TriggerReason {
	case "amount_threshold":
		reasonText = fmt.Sprintf("Amount exceeds approval threshold (%.2f)", amount)
	case "new_vendor":
		reasonText = fmt.Sprintf("New vendor '%s' requires approval for first transactions", vendorName)
	case "risk_flag":
		reasonText = "Risk flag detected (possible duplicate)"
	default:
		reasonText = "Manual approval required"
	}

	msg := fmt.Sprintf("Approval required: %s\n\nAn approver has been notified. You can also approve via the web dashboard.", reasonText)

	// Send approval notification with approve/reject buttons.
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			tele.Btn{Unique: btnApprove.Unique, Text: "Approve", Data: approval.ID.String()},
			tele.Btn{Unique: btnReject.Unique, Text: "Reject", Data: approval.ID.String()},
		),
	)

	_, _ = b.B.Send(c.Chat(), msg, menu)

	// Also try to notify the configured approver via Telegram.
	go b.notifyApprover(companyID, approval.ID, vendorName, amount, reasonText)

	return true
}

// notifyApprover sends a Telegram message to the company's configured approver.
func (b *Bot) notifyApprover(companyID, approvalID uuid.UUID, vendor string, amount float64, reason string) {
	ctx := context.Background()

	settings, err := b.approvals.GetSettings(ctx, companyID)
	if err != nil || settings == nil || !settings.ApproverUserID.Valid {
		return
	}

	// Look up the approver's Telegram chat ID.
	tgUser, err := b.q.GetTelegramUserByUserID(ctx, settings.ApproverUserID.Bytes)
	if err != nil {
		return
	}

	chat := &tele.Chat{ID: tgUser.ChatID}

	msg := fmt.Sprintf("Receipt approval request:\n\nVendor: %s\nAmount: %.2f\nReason: %s", vendor, amount, reason)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			tele.Btn{Unique: btnApprove.Unique, Text: "Approve", Data: approvalID.String()},
			tele.Btn{Unique: btnReject.Unique, Text: "Reject", Data: approvalID.String()},
		),
	)

	if _, err := b.B.Send(chat, msg, menu); err != nil {
		slog.Warn("failed to notify approver via telegram", "error", err)
	}
}

// handleApproveReceipt handles the "Approve" button press.
func (b *Bot) handleApproveReceipt(c tele.Context) error {
	ctx := context.Background()

	data := c.Callback().Data
	approvalID, err := uuid.Parse(data)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid approval ID"})
	}

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Please /link your account first"})
	}

	result, err := b.approvals.Approve(ctx, approvalID, tgUser.UserID, nil)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Approval failed: " + err.Error()})
	}

	// Parse batch results for summary.
	batch, bErr := b.q.GetReceiptBatchByID(ctx, result.BatchID)
	summary := "Receipt"
	if bErr == nil && batch.Results != nil {
		var results []service.ReceiptResult
		if json.Unmarshal(batch.Results, &results) == nil && len(results) > 0 {
			vendor := ""
			if v, ok := results[0].Parsed.VendorName.Value.(string); ok {
				vendor = v
			}
			if vendor != "" {
				summary = vendor
			}
		}
	}

	_, _ = c.Bot().Edit(c.Message(), fmt.Sprintf("Approved: %s", summary))
	return c.Respond(&tele.CallbackResponse{Text: "Approved"})
}

// handleRejectReceipt handles the "Reject" button press.
func (b *Bot) handleRejectReceipt(c tele.Context) error {
	ctx := context.Background()

	data := c.Callback().Data
	approvalID, err := uuid.Parse(data)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid approval ID"})
	}

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Please /link your account first"})
	}

	_, err = b.approvals.Reject(ctx, approvalID, tgUser.UserID, nil)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Rejection failed: " + err.Error()})
	}

	_, _ = c.Bot().Edit(c.Message(), "Receipt rejected.")
	return c.Respond(&tele.CallbackResponse{Text: "Rejected"})
}
