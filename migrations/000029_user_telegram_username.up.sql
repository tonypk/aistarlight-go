ALTER TABLE users ADD COLUMN telegram_username VARCHAR(100);
CREATE UNIQUE INDEX idx_users_telegram_username ON users(telegram_username) WHERE telegram_username IS NOT NULL;
