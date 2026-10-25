-- name: InsertSupportMessage :one
INSERT INTO support_messages (user_id, question, question_norm, preset, method, status, answer, source_ids, model,
  prompt_version, tokens, duration_ms, cached, error)
VALUES (@user_id, @question, @question_norm, sqlc.narg('preset'), @method, @status, @answer, @source_ids, @model,
  @prompt_version, @tokens, @duration_ms, @cached, sqlc.narg('error'))
RETURNING *;

-- Ответы помощника не персональны: проверенный ответ модели на тот же вопрос
-- переиспользуется для всех пользователей в течение недели.
-- name: FindCachedSupportAnswer :one
SELECT * FROM support_messages
WHERE question_norm = @question_norm AND prompt_version = @prompt_version
  AND status = 'succeeded' AND method = 'llm' AND created_at > now() - interval '7 days'
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSupportMessages :many
SELECT * FROM (
  SELECT * FROM support_messages WHERE user_id = @user_id AND status = 'succeeded'
  ORDER BY created_at DESC LIMIT 30
) m ORDER BY created_at;

-- name: CountSupportLLMToday :one
SELECT count(*)::int FROM support_messages
WHERE user_id = @user_id AND method IN ('llm','fallback') AND NOT cached AND created_at >= @since;
