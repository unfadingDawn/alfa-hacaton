-- +goose Up
-- +goose StatementBegin

-- Уточняющие вопросы к готовому персональному анализу (критерий успеха 10).
-- Вопрос всегда относится к анализу конкретного пользователя; ответ строится
-- только по сохранённому анализу и его реестру источников.
CREATE TABLE analysis_questions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id    UUID NOT NULL REFERENCES personal_analyses(id) ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Версия анализа, к которой задан вопрос: sha256 от result. Анализ
    -- пересчитывается upsert-ом с тем же id, поэтому без хеша нельзя понять,
    -- что ответ относится к устаревшей версии.
    analysis_hash  TEXT NOT NULL,
    question       TEXT NOT NULL,
    question_norm  TEXT NOT NULL,                 -- ключ кэша одинаковых вопросов
    preset         TEXT,                          -- код быстрого вопроса или NULL
    method         TEXT NOT NULL CHECK (method IN ('llm','deterministic')),
    status         TEXT NOT NULL CHECK (status IN ('succeeded','failed')),
    answer         JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_ids     TEXT[] NOT NULL DEFAULT '{}',
    model          TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    tokens         INTEGER NOT NULL DEFAULT 0,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    cached         BOOLEAN NOT NULL DEFAULT false, -- ответ взят из кэша (без вызова модели)
    cache_hits     INTEGER NOT NULL DEFAULT 0,     -- сколько раз этот ответ повторно отдан из кэша
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX analysis_questions_analysis_idx ON analysis_questions (analysis_id, created_at);
CREATE INDEX analysis_questions_user_idx ON analysis_questions (user_id, created_at DESC);
CREATE INDEX analysis_questions_cache_idx ON analysis_questions (analysis_hash, question_norm, prompt_version)
    WHERE status = 'succeeded';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS analysis_questions;
-- +goose StatementEnd
