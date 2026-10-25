-- name: CreateRun :one
INSERT INTO analysis_runs (kind, status, user_id, company_id, article_id)
VALUES (@kind, 'running', sqlc.narg('user_id'), sqlc.narg('company_id'), sqlc.narg('article_id'))
RETURNING id;

-- name: FinishRun :exec
UPDATE analysis_runs
SET status = @status, models = @models, prompt_tokens = @prompt_tokens,
    completion_tokens = @completion_tokens, steps = @steps, error = sqlc.narg('error'),
    finished_at = now()
WHERE id = @id;

-- name: GetRun :one
SELECT * FROM analysis_runs WHERE id = @id;

-- Дневной расход токенов: запуски агента, вопросы к анализам и помощник поддержки.
-- name: TokensUsedSince :one
SELECT (
  (SELECT coalesce(sum(prompt_tokens + completion_tokens), 0) FROM analysis_runs WHERE started_at >= @since) +
  (SELECT coalesce(sum(tokens), 0) FROM analysis_questions WHERE created_at >= @since AND NOT cached) +
  (SELECT coalesce(sum(tokens), 0) FROM support_messages WHERE created_at >= @since AND NOT cached)
)::bigint AS tokens;

-- name: UpsertAnalysis :one
INSERT INTO personal_analyses (article_id, user_id, company_id, role_code, depth, method, input_hash,
  relevance_score, importance_level, impact_type, matched_topics, why_it_matters, result, run_id)
VALUES (@article_id, @user_id, @company_id, @role_code, @depth, @method, @input_hash,
  @relevance_score, @importance_level, @impact_type, @matched_topics, @why_it_matters, @result, sqlc.narg('run_id'))
ON CONFLICT (article_id, user_id, depth) DO UPDATE SET
  company_id = EXCLUDED.company_id, role_code = EXCLUDED.role_code, method = EXCLUDED.method,
  input_hash = EXCLUDED.input_hash, relevance_score = EXCLUDED.relevance_score,
  importance_level = EXCLUDED.importance_level, impact_type = EXCLUDED.impact_type,
  matched_topics = EXCLUDED.matched_topics, why_it_matters = EXCLUDED.why_it_matters,
  result = EXCLUDED.result, run_id = EXCLUDED.run_id, created_at = now()
RETURNING *;

-- name: GetAnalysis :one
SELECT * FROM personal_analyses
WHERE article_id = @article_id AND user_id = @user_id AND depth = @depth;

-- Готовый анализ той же статьи для той же компании и роли (кэш между пользователями).
-- name: FindCachedFullAnalysis :one
SELECT * FROM personal_analyses
WHERE article_id = @article_id AND depth = 'full' AND method = 'llm' AND input_hash = @input_hash
ORDER BY created_at DESC
LIMIT 1;

-- name: DeleteUserRuleAnalyses :exec
DELETE FROM personal_analyses WHERE user_id = @user_id AND method = 'rules';

-- name: DeleteUserBriefAnalyses :exec
DELETE FROM personal_analyses WHERE user_id = @user_id AND depth = 'brief';

-- name: UpsertFeedback :exec
INSERT INTO feedback (user_id, article_id, analysis_id, rating, comment)
VALUES (@user_id, @article_id, sqlc.narg('analysis_id'), @rating, @comment)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  analysis_id = EXCLUDED.analysis_id, rating = EXCLUDED.rating, comment = EXCLUDED.comment, created_at = now();

-- name: GetFeedback :one
SELECT * FROM feedback WHERE user_id = @user_id AND article_id = @article_id;
