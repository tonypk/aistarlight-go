-- name: GetAccountBySubType :one
SELECT * FROM accounts
WHERE company_id = $1 AND sub_type = $2 AND is_active = true
LIMIT 1;

-- name: GetMatchedTransactionsWithoutJournal :many
SELECT * FROM transactions
WHERE company_id = $1
  AND match_status IN ('matched', 'split_matched')
  AND journal_entry_id IS NULL
ORDER BY date ASC, row_index ASC;
