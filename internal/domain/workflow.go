package domain

type ReportStatus string

const (
	StatusDraft    ReportStatus = "draft"
	StatusReview   ReportStatus = "review"
	StatusApproved ReportStatus = "approved"
	StatusRejected ReportStatus = "rejected"
	StatusFiled    ReportStatus = "filed"
	StatusArchived ReportStatus = "archived"
)

var validTransitions = map[ReportStatus][]ReportStatus{
	StatusDraft:    {StatusReview},
	StatusReview:   {StatusApproved, StatusRejected, StatusDraft},
	StatusApproved: {StatusFiled, StatusReview},
	StatusRejected: {StatusDraft},
	StatusFiled:    {StatusArchived},
	StatusArchived: {},
}

var editableStatuses = map[ReportStatus]bool{
	StatusDraft:    true,
	StatusReview:   true,
	StatusRejected: true,
}

func IsValidTransition(from, to ReportStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func IsEditable(status ReportStatus) bool {
	return editableStatuses[status]
}
