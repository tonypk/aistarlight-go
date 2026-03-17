-- Integration source registration (one per linked HR system per company).
CREATE TABLE integration_sources (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID        NOT NULL REFERENCES companies(id),
    source_system     VARCHAR(50) NOT NULL DEFAULT 'aigonhr',
    remote_company_id VARCHAR(100) NOT NULL,
    api_key_hash      VARCHAR(500) NOT NULL,
    webhook_secret    VARCHAR(500) NOT NULL,
    status            VARCHAR(20) NOT NULL DEFAULT 'active',
    last_event_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, source_system)
);

-- Inbound event inbox for idempotent processing.
CREATE TABLE integration_event_inbox (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID         NOT NULL REFERENCES companies(id),
    source_system  VARCHAR(50)  NOT NULL,
    event_id       VARCHAR(100) NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB        NOT NULL,
    status         VARCHAR(20)  NOT NULL DEFAULT 'received',
    error_message  TEXT,
    processed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(source_system, event_id)
);

CREATE INDEX idx_event_inbox_pending
    ON integration_event_inbox(status, created_at)
    WHERE status IN ('received', 'failed');

CREATE INDEX idx_event_inbox_company
    ON integration_event_inbox(company_id, created_at DESC);
