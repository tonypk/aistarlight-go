CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    notification_type VARCHAR(50) NOT NULL, -- deadline_7day, deadline_3day, deadline_overdue
    title VARCHAR(200) NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ,
    -- Dedup key: prevent duplicate notifications for same company+type+form+period
    dedup_key VARCHAR(200),
    UNIQUE(company_id, dedup_key)
);

CREATE INDEX idx_notifications_company_unread ON notifications(company_id, is_read) WHERE NOT is_read;
CREATE INDEX idx_notifications_company_created ON notifications(company_id, created_at DESC);
