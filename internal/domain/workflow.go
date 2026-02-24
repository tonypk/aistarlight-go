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

func IsValidTransition(from, to string) bool {
	allowed, ok := validTransitions[ReportStatus(from)]
	if !ok {
		return false
	}
	target := ReportStatus(to)
	for _, s := range allowed {
		if s == target {
			return true
		}
	}
	return false
}

func IsEditable(status string) bool {
	return editableStatuses[ReportStatus(status)]
}
