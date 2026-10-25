-- name: InsertAnalysisQuestion :one
INSERT INTO analysis_questions (analysis_id, user_id, analysis_hash, question, question_norm, preset, method, status,
  answer, source_ids, model, prompt_version, prompt_tokens, completion_tokens, tokens, duration_ms, cached, error)
VALUES (@analysis_id, @user_id, @analysis_hash, @question, @question_norm, sqlc.narg('preset'), @method, @status,
  @answer, @source_ids, @model, @prompt_version, @prompt_tokens, @completion_tokens, @tokens, @duration_ms, @cached,
  sqlc.narg('error'))
RETURNING *;

-- Готовый ответ на тот же вопрос к той же версии анализа (кэш).
-- Версия анализа (analysis_hash) — хеш результата, поэтому ответ можно
-- переиспользовать и для коллеги, получившего тот же анализ из кэша.
-- name: FindCachedAnswer :one
SELECT * FROM analysis_questions
WHERE analysis_hash = @analysis_hash AND question_norm = @question_norm AND prompt_version = @prompt_version
  AND status = 'succeeded'
ORDER BY (user_id = @user_id) DESC, created_at DESC
LIMIT 1;

-- name: TouchCachedAnswer :exec
UPDATE analysis_questions SET cache_hits = cache_hits + 1 WHERE id = @id;

-- name: ListAnalysisQuestions :many
SELECT * FROM analysis_questions
WHERE analysis_id = @analysis_id AND user_id = @user_id AND status = 'succeeded'
ORDER BY created_at
LIMIT 100;

-- Лимиты считаются только по реальным обращениям к модели (включая неудачные):
-- ответы из кэша и детерминированные ответы бюджет не расходуют.
-- name: CountLLMQuestions :one
SELECT count(*) FILTER (WHERE analysis_id = @analysis_id)::int AS for_analysis,
       count(*) FILTER (WHERE created_at >= @since)::int AS for_user_today
FROM analysis_questions
WHERE user_id = @user_id AND method = 'llm' AND NOT cached;
