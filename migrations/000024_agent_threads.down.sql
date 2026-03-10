DROP INDEX IF EXISTS idx_agent_action_logs_company;
DROP INDEX IF EXISTS idx_agent_action_logs_thread;
DROP TABLE IF EXISTS agent_action_logs;

DROP INDEX IF EXISTS idx_chat_messages_thread;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS action_results_json;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS citations_json;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS message_type;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS agent_id;
ALTER TABLE chat_messages DROP COLUMN IF EXISTS thread_id;

DROP INDEX IF EXISTS idx_agent_threads_lookup;
DROP INDEX IF EXISTS idx_agent_threads_company;
DROP TABLE IF EXISTS agent_threads;
