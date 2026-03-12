DROP INDEX IF EXISTS idx_users_telegram_username;
ALTER TABLE users DROP COLUMN IF EXISTS telegram_username;
