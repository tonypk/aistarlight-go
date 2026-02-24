package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Data migration tool: Python AIStarlight DB → Go AIStarlight DB
//
// Source schema (Python):
//   tenants → companies (rename + add columns)
//   users (has tenant_id) → users (no tenant_id, separate membership)
//   user_tenants → company_members
//   17 tables with tenant_id → company_id
//
// Usage:
//   go run ./cmd/datamigrate \
//     -source "postgres://...old_db..." \
//     -target "postgres://...new_db..." \
//     [--dry-run] [--verify-only]

func main() {
	sourceURL := flag.String("source", "", "Source (Python) database URL")
	targetURL := flag.String("target", "", "Target (Go) database URL")
	dryRun := flag.Bool("dry-run", false, "Print SQL without executing")
	verifyOnly := flag.Bool("verify-only", false, "Only run verification queries")
	flag.Parse()

	if *sourceURL == "" || *targetURL == "" {
		fmt.Fprintln(os.Stderr, "Usage: datamigrate -source <url> -target <url> [--dry-run] [--verify-only]")
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	src, err := pgxpool.New(ctx, *sourceURL)
	if err != nil {
		slog.Error("connect source DB failed", "error", err)
		os.Exit(1)
	}
	defer src.Close()

	dst, err := pgxpool.New(ctx, *targetURL)
	if err != nil {
		slog.Error("connect target DB failed", "error", err)
		os.Exit(1)
	}
	defer dst.Close()

	m := &migrator{src: src, dst: dst, dryRun: *dryRun}

	if *verifyOnly {
		if err := m.verify(ctx); err != nil {
			slog.Error("verification failed", "error", err)
			os.Exit(1)
		}
		slog.Info("verification passed")
		return
	}

	slog.Info("starting data migration", "dry_run", *dryRun)
	start := time.Now()

	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"migrate tenants → companies", m.migrateTenants},
		{"migrate users", m.migrateUsers},
		{"migrate user_tenants → company_members", m.migrateUserTenants},
		{"migrate reports", m.migrateTableSimple("reports")},
		{"migrate reconciliation_sessions", m.migrateTableSimple("reconciliation_sessions")},
		{"migrate transactions", m.migrateTableSimple("transactions")},
		{"migrate suppliers", m.migrateTableSimple("suppliers")},
		{"migrate chat_messages", m.migrateTableSimple("chat_messages")},
		{"migrate user_preferences", m.migrateTableSimple("user_preferences")},
		{"migrate correction_history", m.migrateTableSimple("correction_history")},
		{"migrate audit_logs", m.migrateTableSimple("audit_logs")},
		{"migrate anomalies", m.migrateTableSimple("anomalies")},
		{"migrate withholding_certificates", m.migrateTableSimple("withholding_certificates")},
		{"migrate receipt_batches", m.migrateTableSimple("receipt_batches")},
		{"migrate bank_reconciliation_batches", m.migrateTableSimple("bank_reconciliation_batches")},
		{"migrate corrections", m.migrateTableSimple("corrections")},
		{"migrate correction_rules", m.migrateTableSimple("correction_rules")},
		{"migrate validation_results", m.migrateTableSimple("validation_results")},
		{"migrate knowledge_chunks", m.migrateKnowledgeChunks},
		{"migrate form_schemas", m.migrateFormSchemas},
		{"migrate revoked_tokens", m.migrateRevokedTokens},
		{"verify", m.verify},
	}

	for i, step := range steps {
		slog.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(steps), step.name))
		if err := step.fn(ctx); err != nil {
			slog.Error("migration step failed", "step", step.name, "error", err)
			os.Exit(1)
		}
	}

	slog.Info("migration complete", "duration", time.Since(start).Round(time.Second))
}
