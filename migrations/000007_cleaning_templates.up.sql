-- Cleaning templates store AI results for reuse when the same Excel format is uploaded again.
CREATE TABLE cleaning_templates (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id       UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    signature_hash   VARCHAR(64) NOT NULL,
    header_signature JSONB NOT NULL,
    ai_result        JSONB NOT NULL,
    hit_count        INT NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, signature_hash)
);

CREATE INDEX idx_cleaning_templates_company ON cleaning_templates(company_id);
CREATE INDEX idx_cleaning_templates_hash ON cleaning_templates(company_id, signature_hash);
