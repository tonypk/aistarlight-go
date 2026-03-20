CREATE TABLE action_plans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    thread_id UUID NOT NULL REFERENCES agent_threads(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    company_id UUID NOT NULL REFERENCES companies(id),
    user_id UUID NOT NULL REFERENCES users(id),
    tool_name TEXT NOT NULL,
    tool_args JSONB NOT NULL DEFAULT '{}',
    summary TEXT NOT NULL DEFAULT '',
    impact JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'confirmed', 'cancelled', 'executed', 'failed', 'timeout')),
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ
);

CREATE INDEX idx_action_plans_thread ON action_plans(thread_id);
CREATE INDEX idx_action_plans_pending ON action_plans(status) WHERE status = 'pending';
CREATE INDEX idx_action_plans_company ON action_plans(company_id);
