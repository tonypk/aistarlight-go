-- Vendor posting policies: per-vendor default account/tax/department/project mappings
-- that learn from user confirmations and corrections.

CREATE TABLE vendor_posting_policies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vendor_normalized TEXT NOT NULL,
    aliases JSONB DEFAULT '[]'::jsonb,
    default_category TEXT,
    default_account_code TEXT,
    default_tax_code TEXT,
    default_department TEXT,
    default_project TEXT,
    usage_count INT NOT NULL DEFAULT 0,
    accept_count INT NOT NULL DEFAULT 0,
    correction_count INT NOT NULL DEFAULT 0,
    confidence_score NUMERIC(5,4) NOT NULL DEFAULT 0.5000,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, vendor_normalized)
);

CREATE INDEX idx_vpp_company ON vendor_posting_policies(company_id);
CREATE INDEX idx_vpp_company_vendor ON vendor_posting_policies(company_id, vendor_normalized);
CREATE INDEX idx_vpp_aliases ON vendor_posting_policies USING gin(aliases);
CREATE INDEX idx_vpp_confidence ON vendor_posting_policies(company_id, confidence_score DESC);

-- Extend transactions with posting detail fields
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS account_code TEXT,
    ADD COLUMN IF NOT EXISTS tax_code TEXT,
    ADD COLUMN IF NOT EXISTS department TEXT,
    ADD COLUMN IF NOT EXISTS project TEXT;
