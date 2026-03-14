-- name: CreateExchangeRate :one
INSERT INTO exchange_rates (id, company_id, from_currency, to_currency, rate, effective_date, source, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
RETURNING *;

-- name: GetExchangeRateByID :one
SELECT * FROM exchange_rates WHERE id = $1;

-- name: GetLatestExchangeRate :one
SELECT * FROM exchange_rates
WHERE company_id = $1 AND from_currency = $2 AND to_currency = $3
  AND effective_date <= $4
ORDER BY effective_date DESC
LIMIT 1;

-- name: ListExchangeRatesByCompany :many
SELECT * FROM exchange_rates
WHERE company_id = $1
ORDER BY effective_date DESC, from_currency ASC
LIMIT $2 OFFSET $3;

-- name: CountExchangeRatesByCompany :one
SELECT COUNT(*) FROM exchange_rates WHERE company_id = $1;

-- name: DeleteExchangeRate :exec
DELETE FROM exchange_rates WHERE id = $1 AND company_id = $2;
