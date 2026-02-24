-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";

-- ============================================================
-- Layer 1: Organizations (代账公司 / 企业服务公司)
-- ============================================================
CREATE TABLE organizations (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(200) NOT NULL,
    slug            VARCHAR(100) UNIQUE NOT NULL,
    plan            VARCHAR(20) NOT NULL DEFAULT 'free',
    max_companies   INT NOT NULL DEFAULT 5,
    max_users       INT NOT NULL DEFAULT 10,
    settings        JSONB NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Layer 2: Companies (企业，可属于 org 或独立)
-- ============================================================
CREATE TABLE companies (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id     UUID REFERENCES organizations(id) ON DELETE SET NULL,
    company_name        VARCHAR(200) NOT NULL,
    tin_number          VARCHAR(20),
    rdo_code            VARCHAR(10),
    vat_classification  VARCHAR(20) NOT NULL DEFAULT 'vat_registered',
    fiscal_year_end     VARCHAR(5) NOT NULL DEFAULT '12-31',
    industry            VARCHAR(50),
    address             TEXT,
    plan                VARCHAR(20) NOT NULL DEFAULT 'free',
    settings            JSONB NOT NULL DEFAULT '{}',
    is_active           BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_companies_org ON companies(organization_id) WHERE organization_id IS NOT NULL;

-- ============================================================
-- Layer 3: Users
-- ============================================================
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    hashed_password VARCHAR(255) NOT NULL,
    full_name       VARCHAR(100),
    api_key         VARCHAR(64) UNIQUE,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Organization membership
CREATE TABLE org_members (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            VARCHAR(20) NOT NULL DEFAULT 'org_member',
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id)
);

-- Company membership
CREATE TABLE company_members (
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       VARCHAR(20) NOT NULL DEFAULT 'viewer',
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (company_id, user_id)
);

-- ============================================================
-- Reports
-- ============================================================
CREATE TABLE reports (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id              UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    report_type             VARCHAR(20) NOT NULL,
    period                  VARCHAR(20) NOT NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'draft',
    input_data              JSONB,
    calculated_data         JSONB,
    file_path               VARCHAR(500),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at            TIMESTAMPTZ,
    created_by              UUID REFERENCES users(id),
    updated_by              UUID REFERENCES users(id),
    updated_at              TIMESTAMPTZ,
    version                 INT NOT NULL DEFAULT 1,
    overrides               JSONB,
    original_calculated_data JSONB,
    notes                   TEXT,
    compliance_score        INT
);

CREATE INDEX idx_reports_company ON reports(company_id);
CREATE INDEX idx_reports_type_period ON reports(report_type, period);

-- ============================================================
-- Reconciliation Sessions
-- ============================================================
CREATE TABLE reconciliation_sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    created_by      UUID NOT NULL REFERENCES users(id),
    period          VARCHAR(20) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    report_id       UUID REFERENCES reports(id),
    source_files    JSONB DEFAULT '[]',
    summary         JSONB,
    reconciliation_result JSONB,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_recon_sessions_company ON reconciliation_sessions(company_id);

-- ============================================================
-- Suppliers
-- ============================================================
CREATE TABLE suppliers (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id          UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    tin                 VARCHAR(20) NOT NULL,
    name                VARCHAR(200) NOT NULL,
    address             TEXT,
    supplier_type       VARCHAR(20) NOT NULL DEFAULT 'corporation',
    default_ewt_rate    NUMERIC(5,4),
    default_atc_code    VARCHAR(20),
    is_vat_registered   BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_suppliers_company ON suppliers(company_id);

-- ============================================================
-- Transactions
-- ============================================================
CREATE TABLE transactions (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id              UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    session_id              UUID NOT NULL REFERENCES reconciliation_sessions(id) ON DELETE CASCADE,
    source_type             VARCHAR(20) NOT NULL,
    source_file_id          VARCHAR(36) NOT NULL,
    row_index               INT NOT NULL,
    date                    DATE,
    description             TEXT,
    amount                  NUMERIC(15,2) NOT NULL,
    vat_amount              NUMERIC(15,2) NOT NULL DEFAULT 0,
    vat_type                VARCHAR(20) NOT NULL DEFAULT 'vatable',
    category                VARCHAR(20) NOT NULL DEFAULT 'goods',
    tin                     VARCHAR(20),
    confidence              NUMERIC(3,2) NOT NULL DEFAULT 0,
    classification_source   VARCHAR(20) NOT NULL DEFAULT 'ai',
    raw_data                JSONB,
    match_group_id          UUID,
    match_status            VARCHAR(20) NOT NULL DEFAULT 'unmatched',
    ewt_rate                NUMERIC(5,4),
    ewt_amount              NUMERIC(15,2),
    atc_code                VARCHAR(20),
    supplier_id             UUID REFERENCES suppliers(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_company ON transactions(company_id);
CREATE INDEX idx_transactions_session ON transactions(session_id);

-- ============================================================
-- Chat Messages
-- ============================================================
CREATE TABLE chat_messages (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),
    role        VARCHAR(20) NOT NULL,
    content     TEXT NOT NULL,
    tool_calls  JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_company ON chat_messages(company_id);

-- ============================================================
-- User Preferences (long-term memory)
-- ============================================================
CREATE TABLE user_preferences (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    report_type     VARCHAR(20) NOT NULL,
    column_mappings JSONB NOT NULL DEFAULT '{}',
    format_rules    JSONB NOT NULL DEFAULT '{}',
    auto_fill_rules JSONB NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, report_type)
);

-- ============================================================
-- Correction History (legacy, kept for migration)
-- ============================================================
CREATE TABLE correction_history (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    report_type VARCHAR(20) NOT NULL,
    field_name  VARCHAR(100),
    old_value   TEXT,
    new_value   TEXT,
    reason      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Knowledge Chunks (RAG)
-- ============================================================
CREATE TABLE knowledge_chunks (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source      VARCHAR(500),
    category    VARCHAR(50),
    content     TEXT NOT NULL,
    embedding   vector(1024),
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Form Schemas
-- ============================================================
CREATE TABLE form_schemas (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    form_type         VARCHAR(30) UNIQUE NOT NULL,
    version           INT NOT NULL DEFAULT 1,
    name              VARCHAR(200) NOT NULL,
    frequency         VARCHAR(20) NOT NULL,
    is_active         BOOLEAN NOT NULL DEFAULT true,
    schema_def        JSONB NOT NULL,
    calculation_rules JSONB NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Audit Logs
-- ============================================================
CREATE TABLE audit_logs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id     UUID REFERENCES users(id),
    entity_type VARCHAR(50) NOT NULL,
    entity_id   UUID,
    action      VARCHAR(50) NOT NULL,
    changes     JSONB,
    comment     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_company ON audit_logs(company_id);

-- ============================================================
-- Anomalies
-- ============================================================
CREATE TABLE anomalies (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    session_id      UUID NOT NULL REFERENCES reconciliation_sessions(id) ON DELETE CASCADE,
    transaction_id  UUID REFERENCES transactions(id),
    anomaly_type    VARCHAR(30) NOT NULL,
    severity        VARCHAR(10) NOT NULL,
    description     TEXT NOT NULL,
    details         JSONB,
    status          VARCHAR(20) NOT NULL DEFAULT 'open',
    resolved_by     UUID REFERENCES users(id),
    resolved_at     TIMESTAMPTZ,
    resolution_note TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_anomalies_session ON anomalies(session_id);

-- ============================================================
-- Withholding Certificates (BIR 2307)
-- ============================================================
CREATE TABLE withholding_certificates (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    session_id      UUID REFERENCES reconciliation_sessions(id),
    supplier_id     UUID NOT NULL REFERENCES suppliers(id),
    period          VARCHAR(20) NOT NULL,
    quarter         VARCHAR(6) NOT NULL,
    atc_code        VARCHAR(20) NOT NULL,
    income_type     VARCHAR(100) NOT NULL,
    income_amount   NUMERIC(15,2) NOT NULL,
    ewt_rate        NUMERIC(5,4) NOT NULL,
    tax_withheld    NUMERIC(15,2) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    file_path       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_wh_certs_company ON withholding_certificates(company_id);

-- ============================================================
-- Corrections (Phase 3)
-- ============================================================
CREATE TABLE corrections (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id),
    entity_type     VARCHAR(30) NOT NULL,
    entity_id       UUID NOT NULL,
    field_name      VARCHAR(100) NOT NULL,
    old_value       VARCHAR(500),
    new_value       VARCHAR(500) NOT NULL,
    reason          VARCHAR(500),
    context_data    JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_corrections_company ON corrections(company_id);

-- ============================================================
-- Correction Rules (learned)
-- ============================================================
CREATE TABLE correction_rules (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id              UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    rule_type               VARCHAR(30) NOT NULL,
    match_criteria          JSONB NOT NULL,
    correction_field        VARCHAR(100) NOT NULL,
    correction_value        VARCHAR(100) NOT NULL,
    confidence              NUMERIC(3,2) NOT NULL DEFAULT 0.85,
    source_correction_count INT NOT NULL DEFAULT 0,
    is_active               BOOLEAN NOT NULL DEFAULT true,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_correction_rules_company ON correction_rules(company_id);

-- ============================================================
-- Validation Results
-- ============================================================
CREATE TABLE validation_results (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    report_id       UUID NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    overall_score   INT NOT NULL,
    check_results   JSONB NOT NULL,
    rag_findings    JSONB,
    validated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_validation_report ON validation_results(report_id);

-- ============================================================
-- Receipt Batches
-- ============================================================
CREATE TABLE receipt_batches (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    total_images    INT NOT NULL DEFAULT 0,
    processed_count INT NOT NULL DEFAULT 0,
    session_id      UUID REFERENCES reconciliation_sessions(id),
    report_id       UUID REFERENCES reports(id),
    report_type     VARCHAR(20) NOT NULL DEFAULT 'BIR_2550M',
    period          VARCHAR(20) NOT NULL,
    results         JSONB DEFAULT '[]',
    error_message   VARCHAR(500),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_receipt_company ON receipt_batches(company_id);

-- ============================================================
-- Bank Reconciliation Batches
-- ============================================================
CREATE TABLE bank_reconciliation_batches (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id          UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    created_by          UUID NOT NULL REFERENCES users(id),
    session_id          UUID REFERENCES reconciliation_sessions(id),
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',
    source_files        JSONB,
    total_entries       INT NOT NULL DEFAULT 0,
    parse_summary       JSONB,
    match_result        JSONB,
    ai_suggestions      JSONB,
    ai_explanations     JSONB,
    amount_tolerance    NUMERIC(10,4) NOT NULL DEFAULT 0.01,
    date_tolerance_days INT NOT NULL DEFAULT 3,
    period              VARCHAR(20) NOT NULL,
    error_message       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bank_recon_company ON bank_reconciliation_batches(company_id);

-- ============================================================
-- Revoked Tokens
-- ============================================================
CREATE TABLE revoked_tokens (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    jti         VARCHAR(36) UNIQUE NOT NULL,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    revoked_at  TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_revoked_jti ON revoked_tokens(jti);
