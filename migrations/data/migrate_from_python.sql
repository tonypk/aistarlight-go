-- ============================================================
-- AIStarlight: Python DB → Go DB Data Migration
-- ============================================================
-- Prerequisites:
--   1. Go DB schema created (000001 + 000002 migrations applied)
--   2. Python DB accessible via postgres_fdw or dblink
--   3. Run on the TARGET (Go) database
--
-- Usage with dblink:
--   1. CREATE EXTENSION IF NOT EXISTS dblink;
--   2. Replace 'python_db_conn' with actual connection string
--   3. Run this script in a transaction
-- ============================================================

-- Connection to source Python DB
-- Replace with actual connection string:
-- \set python_conn 'host=localhost dbname=aistarlight_old user=aistarlight password=xxx'

BEGIN;

-- ============================================================
-- Step 1: tenants → companies
-- ============================================================
INSERT INTO companies (id, company_name, tin_number, rdo_code, vat_classification, plan, created_at, updated_at)
SELECT id, company_name, tin_number, rdo_code,
       COALESCE(vat_classification, 'vat_registered'),
       COALESCE(plan, 'free'),
       created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, company_name, tin_number, rdo_code, vat_classification, plan, created_at, updated_at FROM tenants')
AS t(id UUID, company_name VARCHAR, tin_number VARCHAR, rdo_code VARCHAR,
     vat_classification VARCHAR, plan VARCHAR, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Step 2: users (remove tenant_id, keep other fields)
-- ============================================================
INSERT INTO users (id, email, hashed_password, full_name, api_key, is_active, created_at)
SELECT id, email, hashed_password, full_name, api_key, COALESCE(is_active, true), created_at
FROM dblink(:'python_conn',
  'SELECT id, email, hashed_password, full_name, api_key, is_active, created_at FROM users')
AS t(id UUID, email VARCHAR, hashed_password VARCHAR, full_name VARCHAR,
     api_key VARCHAR, is_active BOOLEAN, created_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Step 3: users.tenant_id → company_members (primary membership)
-- ============================================================
INSERT INTO company_members (company_id, user_id, role, joined_at)
SELECT tenant_id, id,
       CASE WHEN role IN ('owner', 'admin') THEN 'company_admin'
            WHEN role = 'accountant' THEN 'accountant'
            ELSE 'viewer'
       END,
       created_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, role, created_at FROM users WHERE tenant_id IS NOT NULL')
AS t(id UUID, tenant_id UUID, role VARCHAR, created_at TIMESTAMPTZ)
ON CONFLICT (company_id, user_id) DO NOTHING;

-- ============================================================
-- Step 4: user_tenants → company_members (secondary memberships)
-- ============================================================
INSERT INTO company_members (company_id, user_id, role, joined_at)
SELECT tenant_id, user_id,
       CASE WHEN role IN ('owner', 'admin') THEN 'company_admin'
            WHEN role = 'accountant' THEN 'accountant'
            ELSE 'viewer'
       END,
       joined_at
FROM dblink(:'python_conn',
  'SELECT user_id, tenant_id, role, joined_at FROM user_tenants')
AS t(user_id UUID, tenant_id UUID, role VARCHAR, joined_at TIMESTAMPTZ)
ON CONFLICT (company_id, user_id) DO NOTHING;

-- ============================================================
-- Step 5: Business data tables (tenant_id → company_id)
-- ============================================================

-- reports
INSERT INTO reports (id, company_id, report_type, period, status, input_data, calculated_data,
                     file_path, created_at, confirmed_at, created_by, updated_by, updated_at,
                     version, overrides, original_calculated_data, notes, compliance_score)
SELECT id, tenant_id, report_type, period, status, input_data, calculated_data,
       file_path, created_at, confirmed_at, created_by, updated_by, updated_at,
       version, overrides, original_calculated_data, notes, compliance_score
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, report_type, period, status, input_data, calculated_data,
          file_path, created_at, confirmed_at, created_by, updated_by, updated_at,
          version, overrides, original_calculated_data, notes, compliance_score
   FROM reports')
AS t(id UUID, tenant_id UUID, report_type VARCHAR, period VARCHAR, status VARCHAR,
     input_data JSONB, calculated_data JSONB, file_path VARCHAR, created_at TIMESTAMPTZ,
     confirmed_at TIMESTAMPTZ, created_by UUID, updated_by UUID, updated_at TIMESTAMPTZ,
     version INT, overrides JSONB, original_calculated_data JSONB, notes TEXT, compliance_score INT)
ON CONFLICT (id) DO NOTHING;

-- suppliers
INSERT INTO suppliers (id, company_id, tin, name, address, supplier_type,
                       default_ewt_rate, default_atc_code, is_vat_registered, created_at, updated_at)
SELECT id, tenant_id, tin, name, address, supplier_type,
       default_ewt_rate, default_atc_code, is_vat_registered, created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, tin, name, address, supplier_type,
          default_ewt_rate, default_atc_code, is_vat_registered, created_at, updated_at
   FROM suppliers')
AS t(id UUID, tenant_id UUID, tin VARCHAR, name VARCHAR, address TEXT, supplier_type VARCHAR,
     default_ewt_rate NUMERIC, default_atc_code VARCHAR, is_vat_registered BOOLEAN,
     created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- reconciliation_sessions
INSERT INTO reconciliation_sessions (id, company_id, created_by, period, status, report_id,
                                     source_files, summary, reconciliation_result,
                                     completed_at, created_at, updated_at)
SELECT id, tenant_id, created_by, period, status, report_id,
       source_files, summary, reconciliation_result, completed_at, created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, created_by, period, status, report_id,
          source_files, summary, reconciliation_result, completed_at, created_at, updated_at
   FROM reconciliation_sessions')
AS t(id UUID, tenant_id UUID, created_by UUID, period VARCHAR, status VARCHAR, report_id UUID,
     source_files JSONB, summary JSONB, reconciliation_result JSONB,
     completed_at TIMESTAMPTZ, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- transactions
INSERT INTO transactions (id, company_id, session_id, source_type, source_file_id, row_index,
                          date, description, amount, vat_amount, vat_type, category, tin,
                          confidence, classification_source, raw_data, match_group_id,
                          match_status, ewt_rate, ewt_amount, atc_code, supplier_id,
                          created_at, updated_at)
SELECT id, tenant_id, session_id, source_type, source_file_id, row_index,
       date, description, amount, vat_amount, vat_type, category, tin,
       confidence, classification_source, raw_data, match_group_id,
       match_status, ewt_rate, ewt_amount, atc_code, supplier_id,
       created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, session_id, source_type, source_file_id, row_index,
          date, description, amount, vat_amount, vat_type, category, tin,
          confidence, classification_source, raw_data, match_group_id,
          match_status, ewt_rate, ewt_amount, atc_code, supplier_id,
          created_at, updated_at
   FROM transactions')
AS t(id UUID, tenant_id UUID, session_id UUID, source_type VARCHAR, source_file_id VARCHAR,
     row_index INT, date DATE, description TEXT, amount NUMERIC, vat_amount NUMERIC,
     vat_type VARCHAR, category VARCHAR, tin VARCHAR, confidence NUMERIC,
     classification_source VARCHAR, raw_data JSONB, match_group_id UUID,
     match_status VARCHAR, ewt_rate NUMERIC, ewt_amount NUMERIC, atc_code VARCHAR,
     supplier_id UUID, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- chat_messages
INSERT INTO chat_messages (id, company_id, user_id, role, content, tool_calls, created_at)
SELECT id, tenant_id, user_id, role, content, tool_calls, created_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, user_id, role, content, tool_calls, created_at FROM chat_messages')
AS t(id UUID, tenant_id UUID, user_id UUID, role VARCHAR, content TEXT,
     tool_calls JSONB, created_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- audit_logs
INSERT INTO audit_logs (id, company_id, user_id, entity_type, entity_id, action, changes, comment, created_at)
SELECT id, tenant_id, user_id, entity_type, entity_id, action, changes, comment, created_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, user_id, entity_type, entity_id, action, changes, comment, created_at FROM audit_logs')
AS t(id UUID, tenant_id UUID, user_id UUID, entity_type VARCHAR, entity_id UUID,
     action VARCHAR, changes JSONB, comment TEXT, created_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- anomalies
INSERT INTO anomalies (id, company_id, session_id, transaction_id, anomaly_type, severity,
                       description, details, status, resolved_by, resolved_at, resolution_note,
                       created_at, updated_at)
SELECT id, tenant_id, session_id, transaction_id, anomaly_type, severity,
       description, details, status, resolved_by, resolved_at, resolution_note,
       created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, session_id, transaction_id, anomaly_type, severity,
          description, details, status, resolved_by, resolved_at, resolution_note,
          created_at, updated_at FROM anomalies')
AS t(id UUID, tenant_id UUID, session_id UUID, transaction_id UUID, anomaly_type VARCHAR,
     severity VARCHAR, description TEXT, details JSONB, status VARCHAR, resolved_by UUID,
     resolved_at TIMESTAMPTZ, resolution_note TEXT, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- withholding_certificates
INSERT INTO withholding_certificates (id, company_id, session_id, supplier_id, period, quarter,
                                      atc_code, income_type, income_amount, ewt_rate,
                                      tax_withheld, status, file_path, created_at, updated_at)
SELECT id, tenant_id, session_id, supplier_id, period, quarter,
       atc_code, income_type, income_amount, ewt_rate,
       tax_withheld, status, file_path, created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, session_id, supplier_id, period, quarter,
          atc_code, income_type, income_amount, ewt_rate,
          tax_withheld, status, file_path, created_at, updated_at FROM withholding_certificates')
AS t(id UUID, tenant_id UUID, session_id UUID, supplier_id UUID, period VARCHAR, quarter VARCHAR,
     atc_code VARCHAR, income_type VARCHAR, income_amount NUMERIC, ewt_rate NUMERIC,
     tax_withheld NUMERIC, status VARCHAR, file_path TEXT, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- receipt_batches
INSERT INTO receipt_batches (id, company_id, user_id, status, total_images, processed_count,
                             session_id, report_id, report_type, period, results, error_message,
                             created_at, updated_at)
SELECT id, tenant_id, user_id, status, total_images, processed_count,
       session_id, report_id, report_type, period, results, error_message,
       created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, user_id, status, total_images, processed_count,
          session_id, report_id, report_type, period, results, error_message,
          created_at, updated_at FROM receipt_batches')
AS t(id UUID, tenant_id UUID, user_id UUID, status VARCHAR, total_images INT, processed_count INT,
     session_id UUID, report_id UUID, report_type VARCHAR, period VARCHAR, results JSONB,
     error_message VARCHAR, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- bank_reconciliation_batches
INSERT INTO bank_reconciliation_batches (id, company_id, created_by, session_id, status,
                                         source_files, total_entries, parse_summary, match_result,
                                         ai_suggestions, ai_explanations, amount_tolerance,
                                         date_tolerance_days, period, error_message,
                                         created_at, updated_at)
SELECT id, tenant_id, created_by, session_id, status,
       source_files, total_entries, parse_summary, match_result,
       ai_suggestions, ai_explanations, amount_tolerance,
       date_tolerance_days, period, error_message,
       created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, created_by, session_id, status,
          source_files, total_entries, parse_summary, match_result,
          ai_suggestions, ai_explanations, amount_tolerance,
          date_tolerance_days, period, error_message,
          created_at, updated_at FROM bank_reconciliation_batches')
AS t(id UUID, tenant_id UUID, created_by UUID, session_id UUID, status VARCHAR,
     source_files JSONB, total_entries INT, parse_summary JSONB, match_result JSONB,
     ai_suggestions JSONB, ai_explanations JSONB, amount_tolerance NUMERIC,
     date_tolerance_days INT, period VARCHAR, error_message TEXT,
     created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- corrections
INSERT INTO corrections (id, company_id, user_id, entity_type, entity_id, field_name,
                          old_value, new_value, reason, context_data, created_at)
SELECT id, tenant_id, user_id, entity_type, entity_id, field_name,
       old_value, new_value, reason, context_data, created_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, user_id, entity_type, entity_id, field_name,
          old_value, new_value, reason, context_data, created_at FROM corrections')
AS t(id UUID, tenant_id UUID, user_id UUID, entity_type VARCHAR, entity_id UUID,
     field_name VARCHAR, old_value VARCHAR, new_value VARCHAR, reason VARCHAR,
     context_data JSONB, created_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- correction_rules
INSERT INTO correction_rules (id, company_id, rule_type, match_criteria, correction_field,
                               correction_value, confidence, source_correction_count,
                               is_active, created_at, updated_at)
SELECT id, tenant_id, rule_type, match_criteria, correction_field,
       correction_value, confidence, source_correction_count,
       is_active, created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, rule_type, match_criteria, correction_field,
          correction_value, confidence, source_correction_count,
          is_active, created_at, updated_at FROM correction_rules')
AS t(id UUID, tenant_id UUID, rule_type VARCHAR, match_criteria JSONB,
     correction_field VARCHAR, correction_value VARCHAR, confidence NUMERIC,
     source_correction_count INT, is_active BOOLEAN, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- validation_results
INSERT INTO validation_results (id, report_id, company_id, overall_score, check_results,
                                 rag_findings, validated_at)
SELECT id, report_id, tenant_id, overall_score, check_results, rag_findings, validated_at
FROM dblink(:'python_conn',
  'SELECT id, report_id, tenant_id, overall_score, check_results, rag_findings, validated_at
   FROM validation_results')
AS t(id UUID, report_id UUID, tenant_id UUID, overall_score INT, check_results JSONB,
     rag_findings JSONB, validated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- user_preferences
INSERT INTO user_preferences (id, company_id, report_type, column_mappings, format_rules,
                               auto_fill_rules, updated_at)
SELECT id, tenant_id, report_type, column_mappings, format_rules, auto_fill_rules, updated_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, report_type, column_mappings, format_rules, auto_fill_rules, updated_at
   FROM user_preferences')
AS t(id UUID, tenant_id UUID, report_type VARCHAR, column_mappings JSONB,
     format_rules JSONB, auto_fill_rules JSONB, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- correction_history (legacy)
INSERT INTO correction_history (id, company_id, report_type, field_name, old_value, new_value,
                                 reason, created_at)
SELECT id, tenant_id, report_type, field_name, old_value, new_value, reason, created_at
FROM dblink(:'python_conn',
  'SELECT id, tenant_id, report_type, field_name, old_value, new_value, reason, created_at
   FROM correction_history')
AS t(id UUID, tenant_id UUID, report_type VARCHAR, field_name VARCHAR, old_value TEXT,
     new_value TEXT, reason TEXT, created_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Step 6: Global tables (no tenant_id)
-- ============================================================

-- knowledge_chunks
INSERT INTO knowledge_chunks (id, source, category, content, embedding, metadata, created_at)
SELECT id, source, category, content, embedding, metadata, created_at
FROM dblink(:'python_conn',
  'SELECT id, source, category, content, embedding, metadata, created_at FROM knowledge_chunks')
AS t(id UUID, source VARCHAR, category VARCHAR, content TEXT, embedding vector(1024),
     metadata JSONB, created_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- form_schemas
INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active,
                           schema_def, calculation_rules, created_at, updated_at)
SELECT id, form_type, version, name, frequency, is_active,
       schema_def, calculation_rules, created_at, updated_at
FROM dblink(:'python_conn',
  'SELECT id, form_type, version, name, frequency, is_active,
          schema_def, calculation_rules, created_at, updated_at FROM form_schemas')
AS t(id UUID, form_type VARCHAR, version INT, name VARCHAR, frequency VARCHAR,
     is_active BOOLEAN, schema_def JSONB, calculation_rules JSONB,
     created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- revoked_tokens
INSERT INTO revoked_tokens (id, jti, user_id, revoked_at, expires_at)
SELECT id, jti, user_id, revoked_at, expires_at
FROM dblink(:'python_conn',
  'SELECT id, jti, user_id, revoked_at, expires_at FROM revoked_tokens')
AS t(id UUID, jti VARCHAR, user_id UUID, revoked_at TIMESTAMPTZ, expires_at TIMESTAMPTZ)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Step 7: Verification
-- ============================================================
DO $$
DECLARE
    src_count BIGINT;
    dst_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO dst_count FROM companies;
    RAISE NOTICE 'companies: % rows', dst_count;

    SELECT COUNT(*) INTO dst_count FROM users;
    RAISE NOTICE 'users: % rows', dst_count;

    SELECT COUNT(*) INTO dst_count FROM company_members;
    RAISE NOTICE 'company_members: % rows', dst_count;

    SELECT COUNT(*) INTO dst_count FROM reports;
    RAISE NOTICE 'reports: % rows', dst_count;

    SELECT COUNT(*) INTO dst_count FROM transactions;
    RAISE NOTICE 'transactions: % rows', dst_count;

    SELECT COUNT(*) INTO dst_count FROM reconciliation_sessions;
    RAISE NOTICE 'reconciliation_sessions: % rows', dst_count;
END $$;

COMMIT;
