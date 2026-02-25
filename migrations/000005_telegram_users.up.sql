CREATE TABLE telegram_users (
    telegram_id  BIGINT PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_id   UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    chat_id      BIGINT NOT NULL,
    username     VARCHAR(100),
    full_name    VARCHAR(200),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_telegram_users_user_id ON telegram_users(user_id);
