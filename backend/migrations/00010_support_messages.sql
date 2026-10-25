-- +goose Up
-- +goose StatementBegin

-- Вопросы к ИИ-помощнику поддержки: как пользоваться сервисом. Ответы
-- строятся только по статьям справки и проходят проверку на выдуманные
-- разделы, кнопки и утверждения без опоры на справку.
CREATE TABLE support_messages (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question       TEXT NOT NULL,
    question_norm  TEXT NOT NULL,
    preset         TEXT,
    -- kb — ответ собран из справки без модели; llm — ответ модели прошёл проверку;
    -- fallback — ответ модели отклонён проверкой и заменён статьями справки.
    method         TEXT NOT NULL CHECK (method IN ('kb','llm','fallback')),
    status         TEXT NOT NULL CHECK (status IN ('succeeded','failed')),
    answer         JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_ids     TEXT[] NOT NULL DEFAULT '{}',
    model          TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL,
    tokens         INTEGER NOT NULL DEFAULT 0,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    cached         BOOLEAN NOT NULL DEFAULT false,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX support_messages_user_idx ON support_messages (user_id, created_at DESC);
CREATE INDEX support_messages_cache_idx ON support_messages (question_norm, prompt_version)
    WHERE status = 'succeeded' AND method = 'llm';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS support_messages;
-- +goose StatementEnd
