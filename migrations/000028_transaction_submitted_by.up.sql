ALTER TABLE transactions ADD COLUMN submitted_by UUID REFERENCES users(id);
CREATE INDEX idx_transactions_submitted_by ON transactions (submitted_by);

-- Backfill: for receipt-sourced transactions, copy user_id from receipt_batches
UPDATE transactions t
SET submitted_by = rb.user_id
FROM receipt_batches rb
WHERE t.source_file_id = rb.id::text
  AND t.source_type = 'receipt'
  AND t.submitted_by IS NULL;
