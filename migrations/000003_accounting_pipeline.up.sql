-- ============================================================
-- Chart of Accounts (COA)
-- ============================================================
CREATE TABLE accounts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    account_number  VARCHAR(20) NOT NULL,
    name            VARCHAR(200) NOT NULL,
    account_type    VARCHAR(30) NOT NULL, -- asset, liability, equity, revenue, expense
    sub_type        VARCHAR(50),
    parent_id       UUID REFERENCES accounts(id),
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    is_system       BOOLEAN NOT NULL DEFAULT false,
    normal_balance  VARCHAR(10) NOT NULL DEFAULT 'debit', -- debit, credit
    qbo_account_id  VARCHAR(50),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, account_number)
);

CREATE INDEX idx_accounts_company ON accounts(company_id);
CREATE INDEX idx_accounts_type ON accounts(company_id, account_type);
CREATE INDEX idx_accounts_qbo ON accounts(qbo_account_id) WHERE qbo_account_id IS NOT NULL;

-- ============================================================
-- Accounting Periods
-- ============================================================
CREATE TABLE accounting_periods (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    period_type VARCHAR(20) NOT NULL DEFAULT 'monthly', -- monthly, quarterly, annual
    start_date  DATE NOT NULL,
    end_date    DATE NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'open', -- open, closed, locked
    closed_by   UUID REFERENCES users(id),
    closed_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, start_date, end_date)
);

CREATE INDEX idx_acct_periods_company ON accounting_periods(company_id);
CREATE INDEX idx_acct_periods_dates ON accounting_periods(company_id, start_date, end_date);

-- ============================================================
-- Journal Entries (header)
-- ============================================================
CREATE TABLE journal_entries (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    period_id       UUID REFERENCES accounting_periods(id),
    entry_number    SERIAL,
    entry_date      DATE NOT NULL,
    reference       VARCHAR(100),
    description     TEXT,
    source_type     VARCHAR(30), -- manual, receipt, qbo_sync, reconciliation
    source_id       UUID,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft', -- draft, posted, reversed
    posted_by       UUID REFERENCES users(id),
    posted_at       TIMESTAMPTZ,
    reversed_by_id  UUID REFERENCES journal_entries(id),
    reverses_id     UUID REFERENCES journal_entries(id),
    memo            TEXT,
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_je_company ON journal_entries(company_id);
CREATE INDEX idx_je_date ON journal_entries(company_id, entry_date);
CREATE INDEX idx_je_status ON journal_entries(company_id, status);
CREATE INDEX idx_je_period ON journal_entries(period_id) WHERE period_id IS NOT NULL;
CREATE INDEX idx_je_source ON journal_entries(source_type, source_id) WHERE source_type IS NOT NULL;

-- ============================================================
-- Journal Lines (double-entry rows)
-- ============================================================
CREATE TABLE journal_lines (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    journal_entry_id    UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id          UUID NOT NULL REFERENCES accounts(id),
    line_number         INT NOT NULL DEFAULT 0,
    description         TEXT,
    debit               NUMERIC(15,2) NOT NULL DEFAULT 0,
    credit              NUMERIC(15,2) NOT NULL DEFAULT 0,
    CONSTRAINT chk_debit_or_credit CHECK (
        (debit > 0 AND credit = 0) OR (debit = 0 AND credit > 0)
    )
);

CREATE INDEX idx_jl_entry ON journal_lines(journal_entry_id);
CREATE INDEX idx_jl_account ON journal_lines(account_id);
CREATE INDEX idx_jl_account_entry ON journal_lines(account_id, journal_entry_id);

-- ============================================================
-- QuickBooks Online Connections
-- ============================================================
CREATE TABLE qbo_connections (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id          UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    realm_id            VARCHAR(50) NOT NULL,
    access_token_enc    BYTEA NOT NULL,
    refresh_token_enc   BYTEA NOT NULL,
    token_expiry        TIMESTAMPTZ NOT NULL,
    refresh_expiry      TIMESTAMPTZ NOT NULL,
    scope               VARCHAR(200),
    is_active           BOOLEAN NOT NULL DEFAULT true,
    last_sync_at        TIMESTAMPTZ,
    last_sync_status    VARCHAR(20),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id)
);

-- ============================================================
-- QBO Sync Logs
-- ============================================================
CREATE TABLE qbo_sync_logs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    connection_id   UUID NOT NULL REFERENCES qbo_connections(id) ON DELETE CASCADE,
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    entity_type     VARCHAR(30) NOT NULL, -- accounts, invoices, bills, payments
    sync_type       VARCHAR(20) NOT NULL DEFAULT 'incremental', -- full, incremental
    sync_direction  VARCHAR(10) NOT NULL DEFAULT 'pull', -- pull, push
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    records_synced  INT NOT NULL DEFAULT 0,
    records_failed  INT NOT NULL DEFAULT 0,
    error_details   JSONB,
    status          VARCHAR(20) NOT NULL DEFAULT 'running' -- running, completed, failed
);

CREATE INDEX idx_qbo_sync_company ON qbo_sync_logs(company_id);
CREATE INDEX idx_qbo_sync_conn ON qbo_sync_logs(connection_id);
