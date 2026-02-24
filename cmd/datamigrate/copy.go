package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// copyViaTemp migrates data from source to target via a temp table:
// 1. Read rows from source with selectQuery
// 2. COPY into a temp table on target
// 3. INSERT INTO target from temp table using insertQuery
func (m *migrator) copyViaTemp(ctx context.Context, label, tempTable, insertQuery, selectQuery string) error {
	// Read rows from source.
	srcRows, err := m.src.Query(ctx, selectQuery)
	if err != nil {
		return fmt.Errorf("query source %s: %w", label, err)
	}
	defer srcRows.Close()

	fieldDescs := srcRows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = fd.Name
	}

	var rows [][]interface{}
	for srcRows.Next() {
		vals, err := srcRows.Values()
		if err != nil {
			return fmt.Errorf("scan source row %s: %w", label, err)
		}
		rows = append(rows, vals)
	}
	if err := srcRows.Err(); err != nil {
		return fmt.Errorf("iterate source %s: %w", label, err)
	}

	if len(rows) == 0 {
		slog.Info("no rows to migrate", "table", label)
		return nil
	}

	if m.dryRun {
		slog.Info("dry run", "table", label, "rows", len(rows), "insert_query", insertQuery)
		return nil
	}

	// Create temp table and COPY data into it.
	tx, err := m.dst.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx %s: %w", label, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Create temp table with same structure by copying from source column definitions.
	createTemp := fmt.Sprintf("CREATE TEMP TABLE %s (LIKE %s INCLUDING ALL) ON COMMIT DROP",
		tempTable, guessTargetTable(label))

	// Some temp tables need special handling (source tables don't exist in target).
	if strings.HasPrefix(tempTable, "source_users_with_tenant") {
		createTemp = fmt.Sprintf(`CREATE TEMP TABLE %s (
			id UUID, tenant_id UUID, role VARCHAR(20), created_at TIMESTAMPTZ
		) ON COMMIT DROP`, tempTable)
	} else if tempTable == "source_tenants" {
		createTemp = fmt.Sprintf(`CREATE TEMP TABLE %s (
			id UUID, company_name VARCHAR(200), tin_number VARCHAR(20),
			rdo_code VARCHAR(10), vat_classification VARCHAR(20),
			plan VARCHAR(20), created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
		) ON COMMIT DROP`, tempTable)
	} else if tempTable == "source_user_tenants" {
		createTemp = fmt.Sprintf(`CREATE TEMP TABLE %s (
			user_id UUID, tenant_id UUID, role VARCHAR(20), joined_at TIMESTAMPTZ
		) ON COMMIT DROP`, tempTable)
	}

	if _, err := tx.Exec(ctx, createTemp); err != nil {
		// Fallback: create temp table without LIKE.
		slog.Warn("LIKE failed, trying manual temp table", "table", label, "error", err)
		_ = tx.Rollback(ctx)
		tx, err = m.dst.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx retry %s: %w", label, err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		createManual := buildCreateTemp(tempTable, colNames, fieldDescs)
		if _, err := tx.Exec(ctx, createManual); err != nil {
			return fmt.Errorf("create temp table %s: %w", tempTable, err)
		}
	}

	// COPY data into temp table.
	copyCount, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{tempTable},
		colNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("copy into %s: %w", tempTable, err)
	}

	slog.Info("copied to temp", "table", label, "rows", copyCount)

	// Run the insert query from temp → target.
	tag, err := tx.Exec(ctx, insertQuery)
	if err != nil {
		return fmt.Errorf("insert from temp %s: %w", label, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", label, err)
	}

	slog.Info("migrated", "table", label, "inserted", tag.RowsAffected())
	return nil
}

// getColumns returns column names for a table from the source database.
func (m *migrator) getColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := m.src.Query(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_name = $1 AND table_schema = 'public'
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func joinCols(cols []string) string {
	return strings.Join(cols, ", ")
}

func guessTargetTable(label string) string {
	// Extract target table name from label like "migrate reports" → "reports".
	parts := strings.Fields(label)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return label
}

func buildCreateTemp(name string, colNames []string, fds []pgconn.FieldDescription) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TEMP TABLE %s (", name))
	for i, col := range colNames {
		if i > 0 {
			sb.WriteString(", ")
		}
		// Use TEXT as a safe fallback type for all columns in temp table.
		sb.WriteString(fmt.Sprintf("%s TEXT", col))
	}
	sb.WriteString(") ON COMMIT DROP")
	return sb.String()
}
