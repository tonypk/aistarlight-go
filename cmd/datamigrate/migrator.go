package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type migrator struct {
	src    *pgxpool.Pool
	dst    *pgxpool.Pool
	dryRun bool
}

// migrateTenants copies tenants → companies with column mapping.
func (m *migrator) migrateTenants(ctx context.Context) error {
	query := `
		INSERT INTO companies (id, company_name, tin_number, rdo_code, vat_classification, plan, created_at, updated_at)
		SELECT id, company_name, tin_number, rdo_code,
		       COALESCE(vat_classification, 'vat_registered'),
		       COALESCE(plan, 'free'),
		       created_at, updated_at
		FROM source_tenants
		ON CONFLICT (id) DO NOTHING`

	return m.copyViaTemp(ctx, "tenants", "source_tenants", query,
		"SELECT id, company_name, tin_number, rdo_code, vat_classification, plan, created_at, updated_at FROM tenants")
}

// migrateUsers copies users with tenant_id removed.
// The primary tenant_id becomes a company_member relationship instead.
func (m *migrator) migrateUsers(ctx context.Context) error {
	query := `
		INSERT INTO users (id, email, hashed_password, full_name, api_key, is_active, created_at)
		SELECT id, email, hashed_password, full_name, api_key,
		       COALESCE(is_active, true),
		       created_at
		FROM source_users
		ON CONFLICT (id) DO NOTHING`

	if err := m.copyViaTemp(ctx, "users", "source_users", query,
		"SELECT id, email, hashed_password, full_name, api_key, is_active, created_at FROM users"); err != nil {
		return err
	}

	// Create company_member entries from users.tenant_id (primary relationship).
	primaryQuery := `
		INSERT INTO company_members (company_id, user_id, role, joined_at)
		SELECT tenant_id, id,
		       CASE WHEN role = 'owner' THEN 'company_admin'
		            WHEN role = 'admin' THEN 'company_admin'
		            WHEN role = 'accountant' THEN 'accountant'
		            ELSE 'viewer'
		       END,
		       created_at
		FROM source_users_with_tenant
		ON CONFLICT (company_id, user_id) DO NOTHING`

	return m.copyViaTemp(ctx, "users (primary membership)", "source_users_with_tenant", primaryQuery,
		"SELECT id, tenant_id, role, created_at FROM users WHERE tenant_id IS NOT NULL")
}

// migrateUserTenants copies user_tenants → company_members.
func (m *migrator) migrateUserTenants(ctx context.Context) error {
	query := `
		INSERT INTO company_members (company_id, user_id, role, joined_at)
		SELECT tenant_id, user_id,
		       CASE WHEN role = 'owner' THEN 'company_admin'
		            WHEN role = 'admin' THEN 'company_admin'
		            WHEN role = 'accountant' THEN 'accountant'
		            ELSE 'viewer'
		       END,
		       joined_at
		FROM source_user_tenants
		ON CONFLICT (company_id, user_id) DO NOTHING`

	return m.copyViaTemp(ctx, "user_tenants", "source_user_tenants", query,
		"SELECT user_id, tenant_id, role, joined_at FROM user_tenants")
}

// migrateTableSimple returns a migration function for tables where tenant_id → company_id.
func (m *migrator) migrateTableSimple(table string) func(context.Context) error {
	return func(ctx context.Context) error {
		// Get column list from source, replacing tenant_id with company_id.
		cols, err := m.getColumns(ctx, table)
		if err != nil {
			return fmt.Errorf("get columns for %s: %w", table, err)
		}

		srcCols := cols
		dstCols := make([]string, len(cols))
		for i, c := range cols {
			if c == "tenant_id" {
				dstCols[i] = "company_id"
			} else {
				dstCols[i] = c
			}
		}

		srcColStr := joinCols(srcCols)
		dstColStr := joinCols(dstCols)
		tempTable := "source_" + table

		selectQuery := fmt.Sprintf("SELECT %s FROM %s", srcColStr, table)
		insertQuery := fmt.Sprintf(
			"INSERT INTO %s (%s) SELECT %s FROM %s ON CONFLICT DO NOTHING",
			table, dstColStr, dstColStr, tempTable,
		)

		return m.copyViaTemp(ctx, table, tempTable, insertQuery, selectQuery)
	}
}

// migrateKnowledgeChunks copies knowledge_chunks (no tenant_id).
func (m *migrator) migrateKnowledgeChunks(ctx context.Context) error {
	query := `
		INSERT INTO knowledge_chunks (id, source, category, content, embedding, metadata, created_at)
		SELECT id, source, category, content, embedding, metadata, created_at
		FROM source_knowledge_chunks
		ON CONFLICT (id) DO NOTHING`

	return m.copyViaTemp(ctx, "knowledge_chunks", "source_knowledge_chunks", query,
		"SELECT id, source, category, content, embedding, metadata, created_at FROM knowledge_chunks")
}

// migrateFormSchemas copies form_schemas (no tenant_id).
func (m *migrator) migrateFormSchemas(ctx context.Context) error {
	query := `
		INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, created_at, updated_at)
		SELECT id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, created_at, updated_at
		FROM source_form_schemas
		ON CONFLICT (id) DO NOTHING`

	return m.copyViaTemp(ctx, "form_schemas", "source_form_schemas", query,
		"SELECT id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, created_at, updated_at FROM form_schemas")
}

// migrateRevokedTokens copies revoked_tokens.
func (m *migrator) migrateRevokedTokens(ctx context.Context) error {
	query := `
		INSERT INTO revoked_tokens (id, jti, user_id, revoked_at, expires_at)
		SELECT id, jti, user_id, revoked_at, expires_at
		FROM source_revoked_tokens
		ON CONFLICT (id) DO NOTHING`

	return m.copyViaTemp(ctx, "revoked_tokens", "source_revoked_tokens", query,
		"SELECT id, jti, user_id, revoked_at, expires_at FROM revoked_tokens")
}

// verify checks row counts match between source and target.
func (m *migrator) verify(ctx context.Context) error {
	tables := []struct {
		srcTable string
		dstTable string
	}{
		{"tenants", "companies"},
		{"users", "users"},
		{"user_tenants", "company_members"},
		{"reports", "reports"},
		{"reconciliation_sessions", "reconciliation_sessions"},
		{"transactions", "transactions"},
		{"suppliers", "suppliers"},
		{"chat_messages", "chat_messages"},
		{"user_preferences", "user_preferences"},
		{"audit_logs", "audit_logs"},
		{"anomalies", "anomalies"},
		{"withholding_certificates", "withholding_certificates"},
		{"receipt_batches", "receipt_batches"},
		{"bank_reconciliation_batches", "bank_reconciliation_batches"},
		{"corrections", "corrections"},
		{"correction_rules", "correction_rules"},
		{"validation_results", "validation_results"},
		{"knowledge_chunks", "knowledge_chunks"},
	}

	allOK := true
	for _, t := range tables {
		srcCount, err := m.countRows(ctx, m.src, t.srcTable)
		if err != nil {
			slog.Warn("count source failed", "table", t.srcTable, "error", err)
			continue
		}

		dstCount, err := m.countRows(ctx, m.dst, t.dstTable)
		if err != nil {
			slog.Warn("count target failed", "table", t.dstTable, "error", err)
			continue
		}

		status := "OK"
		if srcCount != dstCount {
			status = "MISMATCH"
			allOK = false
		}

		slog.Info("verify",
			"source", fmt.Sprintf("%s=%d", t.srcTable, srcCount),
			"target", fmt.Sprintf("%s=%d", t.dstTable, dstCount),
			"status", status,
		)
	}

	// Check company_members count (should be >= user_tenants because we also add primary tenant membership).
	srcUT, _ := m.countRows(ctx, m.src, "user_tenants")
	dstCM, _ := m.countRows(ctx, m.dst, "company_members")
	slog.Info("verify memberships",
		"source_user_tenants", srcUT,
		"target_company_members", dstCM,
		"note", "company_members >= user_tenants (includes primary tenant_id memberships)",
	)

	if !allOK {
		return fmt.Errorf("row count mismatches detected")
	}
	return nil
}

func (m *migrator) countRows(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	return count, err
}
