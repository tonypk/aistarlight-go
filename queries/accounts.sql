-- name: CreateAccount :one
INSERT INTO accounts (id, company_id, account_number, name, account_type, sub_type, parent_id, description, is_active, is_system, normal_balance, qbo_account_id, currency_code, default_tax_code, cash_flow_category, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
RETURNING *;

-- name: GetAccountByID :one
SELECT * FROM accounts WHERE id = $1;

-- name: GetAccountByNumber :one
SELECT * FROM accounts WHERE company_id = $1 AND account_number = $2;

-- name: ListAccountsByCompany :many
SELECT * FROM accounts
WHERE company_id = $1 AND is_active = true
ORDER BY account_number ASC
LIMIT $2 OFFSET $3;

-- name: CountAccountsByCompany :one
SELECT COUNT(*) FROM accounts WHERE company_id = $1 AND is_active = true;

-- name: ListAccountsByType :many
SELECT * FROM accounts
WHERE company_id = $1 AND account_type = $2 AND is_active = true
ORDER BY account_number ASC;

-- name: UpdateAccount :exec
UPDATE accounts SET
    name = COALESCE($2, name),
    sub_type = $3,
    description = $4,
    is_active = COALESCE($5, is_active),
    qbo_account_id = $6,
    currency_code = $7,
    default_tax_code = $8,
    cash_flow_category = $9,
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE id = $1 AND is_system = false;

-- name: DeactivateSystemAccountsNotIn :exec
UPDATE accounts SET is_active = false, updated_at = NOW()
WHERE company_id = $1 AND is_system = true AND account_number != ALL($2::text[]);

-- name: GetAccountByQBOID :one
SELECT * FROM accounts WHERE company_id = $1 AND qbo_account_id = $2;

-- name: AccountBalance :one
SELECT
    a.id,
    a.account_number,
    a.name,
    a.account_type,
    a.normal_balance,
    COALESCE(SUM(jl.debit), 0)::NUMERIC(15,2) AS total_debit,
    COALESCE(SUM(jl.credit), 0)::NUMERIC(15,2) AS total_credit
FROM accounts a
LEFT JOIN journal_lines jl ON jl.account_id = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
    AND je.status = 'posted'
    AND je.entry_date <= $2
WHERE a.id = $1
GROUP BY a.id;
