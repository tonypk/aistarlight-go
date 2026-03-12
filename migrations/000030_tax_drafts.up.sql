CREATE TABLE IF NOT EXISTS tax_drafts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    UUID NOT NULL REFERENCES companies(id),
    form_type     VARCHAR(20) NOT NULL,
    period_start  DATE NOT NULL,
    period_end    DATE NOT NULL,
    result        JSONB NOT NULL DEFAULT '{}',
    triggered_by  VARCHAR(50) DEFAULT 'manual',
    created_at    TIMESTAMPTZ DEFAULT now(),
    UNIQUE(company_id, form_type, period_start, period_end)
);

CREATE INDEX idx_tax_drafts_company ON tax_drafts(company_id);
CREATE INDEX idx_tax_drafts_lookup ON tax_drafts(company_id, form_type, period_start, period_end);
