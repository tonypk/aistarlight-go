-- Agent conversation threads (scoped by agent + workflow context)
CREATE TABLE IF NOT EXISTS agent_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id),
    user_id UUID NOT NULL REFERENCES users(id),
    agent_id VARCHAR(32) NOT NULL DEFAULT 'general',
    workflow_type VARCHAR(32),
    entity_type VARCHAR(32),
    entity_id UUID,
    title VARCHAR(255),
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    context_json JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_threads_company ON agent_threads(company_id);
CREATE INDEX IF NOT EXISTS idx_agent_threads_lookup ON agent_threads(company_id, agent_id, entity_type, entity_id);

-- Extend chat_messages with thread + agent info
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS thread_id UUID REFERENCES agent_threads(id);
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS agent_id VARCHAR(32) DEFAULT 'general';
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS message_type VARCHAR(16) DEFAULT 'text';
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS citations_json JSONB;
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS action_results_json JSONB;

CREATE INDEX IF NOT EXISTS idx_chat_messages_thread ON chat_messages(thread_id, created_at);

-- Agent action audit log
CREATE TABLE IF NOT EXISTS agent_action_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id UUID REFERENCES agent_threads(id),
    company_id UUID NOT NULL,
    user_id UUID NOT NULL,
    agent_id VARCHAR(32) NOT NULL,
    action_name VARCHAR(64) NOT NULL,
    action_input JSONB,
    action_result JSONB,
    status VARCHAR(16) NOT NULL DEFAULT 'success',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_action_logs_thread ON agent_action_logs(thread_id);
CREATE INDEX IF NOT EXISTS idx_agent_action_logs_company ON agent_action_logs(company_id, created_at DESC);
