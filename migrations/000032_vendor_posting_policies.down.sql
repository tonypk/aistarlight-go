ALTER TABLE transactions
    DROP COLUMN IF EXISTS project,
    DROP COLUMN IF EXISTS department,
    DROP COLUMN IF EXISTS tax_code,
    DROP COLUMN IF EXISTS account_code;

DROP TABLE IF EXISTS vendor_posting_policies;
