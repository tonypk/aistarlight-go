-- ============================================================
-- AIStarlight: Post-Migration Verification Queries
-- Run on the TARGET (Go) database after migration
-- ============================================================

-- 1. Row counts per table
SELECT 'companies' AS table_name, COUNT(*) AS rows FROM companies
UNION ALL SELECT 'users', COUNT(*) FROM users
UNION ALL SELECT 'company_members', COUNT(*) FROM company_members
UNION ALL SELECT 'org_members', COUNT(*) FROM org_members
UNION ALL SELECT 'organizations', COUNT(*) FROM organizations
UNION ALL SELECT 'reports', COUNT(*) FROM reports
UNION ALL SELECT 'reconciliation_sessions', COUNT(*) FROM reconciliation_sessions
UNION ALL SELECT 'transactions', COUNT(*) FROM transactions
UNION ALL SELECT 'suppliers', COUNT(*) FROM suppliers
UNION ALL SELECT 'chat_messages', COUNT(*) FROM chat_messages
UNION ALL SELECT 'user_preferences', COUNT(*) FROM user_preferences
UNION ALL SELECT 'audit_logs', COUNT(*) FROM audit_logs
UNION ALL SELECT 'anomalies', COUNT(*) FROM anomalies
UNION ALL SELECT 'withholding_certificates', COUNT(*) FROM withholding_certificates
UNION ALL SELECT 'receipt_batches', COUNT(*) FROM receipt_batches
UNION ALL SELECT 'bank_reconciliation_batches', COUNT(*) FROM bank_reconciliation_batches
UNION ALL SELECT 'corrections', COUNT(*) FROM corrections
UNION ALL SELECT 'correction_rules', COUNT(*) FROM correction_rules
UNION ALL SELECT 'validation_results', COUNT(*) FROM validation_results
UNION ALL SELECT 'knowledge_chunks', COUNT(*) FROM knowledge_chunks
UNION ALL SELECT 'form_schemas', COUNT(*) FROM form_schemas
UNION ALL SELECT 'async_tasks', COUNT(*) FROM async_tasks
ORDER BY table_name;

-- 2. Verify every user has at least one company_member entry
SELECT u.id, u.email, u.full_name
FROM users u
LEFT JOIN company_members cm ON cm.user_id = u.id
WHERE cm.user_id IS NULL;
-- Expected: 0 rows (every user should have a company membership)

-- 3. Verify role mapping
SELECT role, COUNT(*) FROM company_members GROUP BY role ORDER BY role;
-- Expected: company_admin, accountant, viewer

-- 4. Verify no orphan reports (company_id must exist)
SELECT r.id, r.company_id
FROM reports r
LEFT JOIN companies c ON c.id = r.company_id
WHERE c.id IS NULL;
-- Expected: 0 rows

-- 5. Verify no orphan transactions
SELECT t.id, t.company_id
FROM transactions t
LEFT JOIN companies c ON c.id = t.company_id
WHERE c.id IS NULL;
-- Expected: 0 rows

-- 6. Check companies without any members
SELECT c.id, c.company_name
FROM companies c
LEFT JOIN company_members cm ON cm.company_id = c.id
WHERE cm.company_id IS NULL;
-- Expected: 0 rows (every company should have at least one member)

-- 7. Data integrity: report counts per company
SELECT c.id, c.company_name, COUNT(r.id) as report_count
FROM companies c
LEFT JOIN reports r ON r.company_id = c.id
GROUP BY c.id, c.company_name
ORDER BY report_count DESC;

-- 8. Verify JWT compatibility (bcrypt hashes preserved)
SELECT id, email, LEFT(hashed_password, 7) as hash_prefix
FROM users LIMIT 5;
-- Expected: hash_prefix should be '$2b$12' or '$2b$10' (bcrypt)
