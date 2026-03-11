ALTER TABLE transactions DROP COLUMN IF EXISTS from_amount;
ALTER TABLE transactions DROP COLUMN IF EXISTS exchange_rate;
ALTER TABLE transactions DROP COLUMN IF EXISTS to_currency;
ALTER TABLE transactions DROP COLUMN IF EXISTS from_currency;
