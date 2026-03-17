# AIStarlight Go Backend

PH/LK/SG accounting & tax compliance — journal entries, tax forms (BIR 2550M/Q, 1601C, 2316, SAWT, etc.), AI column mapping, RAG knowledge base.

## Stack
- Go 1.26, Gin, sqlc (pgx/v5), PostgreSQL, Redis, asynq (task queue), Docker Compose
- Frontend: Vue3 + TS (in `../aistarlight/frontend/`)
- AI: Claude API, pgvector for embeddings

## Build & Deploy
- `make build-linux` → outputs binaries to repo root (NOT `bin/`)
- CRITICAL: `Dockerfile.prebuilt` copies from ROOT (`aistarlight-api`), not `bin/`
- Server: 34.124.185.43 (`ssh aistarlight-gce`)
- MUST restart nginx after rebuilding api/worker (stale container IPs)
- sqlc: `~/go/bin/sqlc generate`

## Key Patterns
- Handler: `struct{svc *service.XxxService}` → routes in `internal/handler/router.go`
- Response: `internal/handler/response` — `OK`, `Created`, `Paginated`, `Err`, `BadRequest`
- Auth: `middleware.GetCompanyID(c)`, `middleware.GetUserID(c)` (UUID-based)
- Domain: `internal/domain/` (32 subdirs), Services: `internal/service/` (108 files)
- Events: `internal/event/` with asynq publisher
- Migrations: `migrations/` (golang-migrate), queries: `queries/`

## Integration with AIGoNHR (HR/Payroll)
- **Pattern**: Webhook receiver → event inbox → process → journal entries / tax forms
- **Tables**: `integration_sources` (source registry), `integration_event_inbox` (idempotent inbox), `gl_mapping_rules` (payroll→GL mapping), `hr_payees` (employee counterparties)
- **Event types**: `payroll.run.completed`, `payroll.run.reversed`, `employee.upserted`, `employee.terminated`
- **Event structs**: `internal/domain/integration/events.go`
- **Queries**: `queries/hr_integration.sql`
- **Webhook endpoint**: POST `/api/v1/webhooks/aigonhr` (verify HMAC signature)
- **GL mapping**: source_dimension (earning/deduction/contribution/net_pay) × source_value → account + debit/credit
- **Payee sync**: `employee.upserted` → upsert `hr_payees` table
- **Journal flow**: payroll event → load GL mappings → build debit/credit lines → create journal entry
- **Tax bridge**: aggregate withholding → auto-fill BIR 1601C/2316 drafts
