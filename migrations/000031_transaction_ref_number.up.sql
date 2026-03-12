ALTER TABLE transactions ADD COLUMN ref_number SERIAL;
CREATE UNIQUE INDEX idx_transactions_company_ref ON transactions(company_id, ref_number);
