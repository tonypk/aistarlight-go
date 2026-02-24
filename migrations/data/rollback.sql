-- ============================================================
-- AIStarlight: Migration Rollback
-- Clears all data from Go DB tables (preserves schema)
-- WARNING: This deletes ALL data in the Go database!
-- ============================================================

BEGIN;

-- Delete in reverse FK order
DELETE FROM async_tasks;
DELETE FROM validation_results;
DELETE FROM correction_rules;
DELETE FROM corrections;
DELETE FROM correction_history;
DELETE FROM bank_reconciliation_batches;
DELETE FROM receipt_batches;
DELETE FROM withholding_certificates;
DELETE FROM anomalies;
DELETE FROM transactions;
DELETE FROM reconciliation_sessions;
DELETE FROM suppliers;
DELETE FROM chat_messages;
DELETE FROM user_preferences;
DELETE FROM audit_logs;
DELETE FROM revoked_tokens;
DELETE FROM company_members;
DELETE FROM org_members;
DELETE FROM reports;
DELETE FROM users;
DELETE FROM companies;
DELETE FROM organizations;
DELETE FROM knowledge_chunks;
DELETE FROM form_schemas;

COMMIT;

-- Verify all empty
SELECT 'companies' AS t, COUNT(*) FROM companies
UNION ALL SELECT 'users', COUNT(*) FROM users
UNION ALL SELECT 'company_members', COUNT(*) FROM company_members
UNION ALL SELECT 'reports', COUNT(*) FROM reports;
