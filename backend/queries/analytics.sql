-- name: TopicDistribution :many
SELECT t.code, t.name, count(DISTINCT a.id)::int AS articles
FROM articles a
JOIN article_topics at ON at.article_id = a.id
JOIN topics t ON t.code = at.topic_code
WHERE a.duplicate_of IS NULL AND a.published_at >= @since
GROUP BY t.code, t.name
ORDER BY articles DESC;

-- name: DailyArticleCounts :many
SELECT date_trunc('day', a.published_at)::timestamptz AS day,
       count(*)::int AS total,
       count(*) FILTER (WHERE pa.importance_level IN ('high','critical'))::int AS important
FROM articles a
LEFT JOIN LATERAL (
  SELECT importance_level FROM personal_analyses x
  WHERE x.article_id = a.id AND x.user_id = @user_id
  ORDER BY (x.depth = 'full') DESC, (x.method = 'llm') DESC LIMIT 1
) pa ON true
WHERE a.duplicate_of IS NULL AND a.published_at >= @since
GROUP BY 1
ORDER BY 1;

-- name: DailyTopicCounts :many
SELECT date_trunc('day', a.published_at)::timestamptz AS day, at.topic_code, count(*)::int AS articles
FROM articles a JOIN article_topics at ON at.article_id = a.id
WHERE a.duplicate_of IS NULL AND a.published_at >= @since
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: UserMatchedTopics :many
SELECT topic, count(*)::int AS hits
FROM (
  SELECT unnest(matched_topics) AS topic
  FROM personal_analyses
  WHERE user_id = @user_id AND method = 'llm' AND relevance_score >= @min_score AND created_at >= @since
) t
GROUP BY topic
ORDER BY hits DESC
LIMIT 15;

-- name: UserImpactCounts :many
SELECT impact_type, importance_level, count(*)::int AS analyses
FROM personal_analyses
WHERE user_id = @user_id AND method = 'llm' AND created_at >= @since
GROUP BY impact_type, importance_level;

-- name: UserTopRisksAndOpportunities :many
SELECT pa.article_id, a.title, pa.importance_level, pa.relevance_score, kind::text AS kind, item->>'title' AS item_title,
       item->>'description' AS item_description
FROM personal_analyses pa
JOIN articles a ON a.id = pa.article_id
CROSS JOIN LATERAL (
  SELECT 'risk' AS kind, r AS item FROM jsonb_array_elements(coalesce(pa.result->'insight'->'risks', '[]'::jsonb)) r
  UNION ALL
  SELECT 'opportunity', o FROM jsonb_array_elements(coalesce(pa.result->'insight'->'opportunities', '[]'::jsonb)) o
) x
WHERE pa.user_id = @user_id AND pa.depth = 'full' AND pa.method = 'llm' AND pa.created_at >= @since
ORDER BY pa.relevance_score DESC, pa.created_at DESC
LIMIT @lim;

-- name: TopicDistributionByMethod :many
SELECT t.code, t.name, at.method, count(DISTINCT a.id)::int AS articles
FROM articles a
JOIN article_topics at ON at.article_id = a.id
JOIN topics t ON t.code = at.topic_code
WHERE a.duplicate_of IS NULL AND a.published_at >= @since
GROUP BY t.code, t.name, at.method
ORDER BY articles DESC;

-- name: DaySummary :one
SELECT
  (SELECT count(*) FROM articles a WHERE a.duplicate_of IS NULL AND a.fetched_at >= @since)::int AS new_articles,
  (SELECT count(*) FROM personal_analyses pa JOIN articles a ON a.id = pa.article_id
     WHERE pa.user_id = @user_id AND pa.depth = 'brief' AND pa.importance_level IN ('high','critical')
       AND a.published_at >= @since)::int AS high_importance,
  (SELECT count(*) FROM personal_analyses pa JOIN articles a ON a.id = pa.article_id
     WHERE pa.user_id = @user_id AND pa.depth = 'brief' AND pa.method = 'llm' AND pa.importance_level IN ('high','critical')
       AND a.published_at >= @since)::int AS high_importance_llm,
  (SELECT count(*) FROM personal_analyses pa
     WHERE pa.user_id = @user_id AND pa.depth = 'full' AND pa.created_at >= @since)::int AS full_analyses;

-- Помесячный срез: как новости отражаются на компании пользователя.
-- Для каждой новости берётся лучшая оценка: полный анализ важнее краткого, модель — правил.
-- name: UserMonthlyExposure :many
SELECT to_char(date_trunc('month', a.published_at AT TIME ZONE @tz::text), 'YYYY-MM')::text AS month,
       count(*)::int AS total,
       count(*) FILTER (WHERE b.relevance_score >= @min_score::int)::int AS relevant,
       count(*) FILTER (WHERE b.relevance_score >= @min_score::int AND b.importance_level = 'critical')::int AS critical,
       count(*) FILTER (WHERE b.relevance_score >= @min_score::int AND b.importance_level = 'high')::int AS high,
       count(*) FILTER (WHERE b.relevance_score >= @min_score::int AND b.importance_level = 'medium')::int AS medium,
       count(*) FILTER (WHERE b.relevance_score >= @min_score::int AND b.importance_level = 'low')::int AS low,
       coalesce(round(avg(b.relevance_score) FILTER (WHERE b.relevance_score >= @min_score::int)), 0)::int AS avg_relevance,
       count(*) FILTER (WHERE b.method = 'llm')::int AS llm_rated,
       count(*) FILTER (WHERE b.method = 'llm' AND b.impact_type = 'risk')::int AS risks,
       count(*) FILTER (WHERE b.method = 'llm' AND b.impact_type = 'opportunity')::int AS opportunities,
       count(*) FILTER (WHERE b.method = 'llm' AND b.impact_type = 'mixed')::int AS mixed,
       coalesce(sum(
         (CASE b.impact_type WHEN 'risk' THEN -1 WHEN 'opportunity' THEN 1 ELSE 0 END) *
         (CASE b.importance_level WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END)
       ) FILTER (WHERE b.method = 'llm'), 0)::int AS net_impact,
       count(*) FILTER (WHERE b.depth = 'full')::int AS full_analyses,
       count(*) FILTER (WHERE b.has_bank_view)::int AS with_bank_view
FROM articles a
LEFT JOIN LATERAL (
  SELECT x.method, x.depth, x.relevance_score, x.importance_level, x.impact_type,
         (x.depth = 'full' AND jsonb_array_length(coalesce(x.result->'insight'->'bank_view', '[]'::jsonb)) > 0) AS has_bank_view
  FROM personal_analyses x
  WHERE x.article_id = a.id AND x.user_id = @user_id
  ORDER BY (x.depth = 'full') DESC, (x.method = 'llm') DESC
  LIMIT 1
) b ON true
WHERE a.duplicate_of IS NULL AND a.published_at >= @since
GROUP BY 1
ORDER BY 1;

-- Каналы влияния: через какие темы значимые новости касаются компании (любая оценка).
-- name: UserExposureChannels :many
SELECT t.code, t.name, count(*)::int AS articles
FROM articles a
JOIN LATERAL (
  SELECT x.relevance_score, x.matched_topics
  FROM personal_analyses x
  WHERE x.article_id = a.id AND x.user_id = @user_id
  ORDER BY (x.depth = 'full') DESC, (x.method = 'llm') DESC
  LIMIT 1
) b ON true
JOIN topics t ON t.code = ANY (b.matched_topics)
WHERE a.duplicate_of IS NULL AND a.published_at >= @since AND b.relevance_score >= @min_score::int
GROUP BY t.code, t.name
ORDER BY articles DESC
LIMIT 10;

-- Техническая стабильность полного агентного анализа текущего пользователя.
-- name: UserQualityRunStats :one
WITH runs AS (
  SELECT * FROM analysis_runs
  WHERE user_id = @user_id AND kind = 'analyze' AND started_at >= @since
)
SELECT count(*)::int AS total,
       count(*) FILTER (WHERE status = 'succeeded')::int AS succeeded,
       count(*) FILTER (WHERE status = 'failed')::int AS failed,
       count(*) FILTER (WHERE error ILIKE '%неверном формате%' OR error ILIKE '%JSON%')::int AS json_errors,
       count(*) FILTER (WHERE error ILIKE '%обрезан%' OR EXISTS (
         SELECT 1 FROM jsonb_array_elements(steps) s WHERE s->>'finish_reason' = 'length'
       ))::int AS truncated,
       count(*) FILTER (WHERE error ILIKE '%время ожидания%' OR error ILIKE '%deadline exceeded%' OR error ILIKE '%timeout%')::int AS timeouts,
       count(*) FILTER (WHERE error ILIKE '%ключ%отклон%' OR error ILIKE '%авторизац%')::int AS auth_errors,
       count(*) FILTER (WHERE error ILIKE '%лимит%' OR error ILIKE '%rate%')::int AS rate_limited,
       coalesce(round(avg(extract(epoch FROM (finished_at - started_at)) * 1000)
         FILTER (WHERE finished_at IS NOT NULL)), 0)::bigint AS avg_latency_ms,
       coalesce(round((percentile_cont(0.95) WITHIN GROUP (
         ORDER BY extract(epoch FROM (finished_at - started_at)) * 1000
       ) FILTER (WHERE finished_at IS NOT NULL))::numeric), 0)::bigint AS p95_latency_ms,
       coalesce(round(avg(prompt_tokens + completion_tokens)), 0)::int AS avg_tokens,
       coalesce(sum(prompt_tokens + completion_tokens), 0)::bigint AS total_tokens,
       coalesce(round(avg(jsonb_array_length(steps))), 0)::int AS avg_steps
FROM runs;

-- name: UserQualityStepStats :one
WITH runs AS (
  SELECT * FROM analysis_runs
  WHERE user_id = @user_id AND kind = 'analyze' AND started_at >= @since
), step_rows AS (
  SELECT s
  FROM runs CROSS JOIN LATERAL jsonb_array_elements(steps) s
)
SELECT count(*) FILTER (WHERE coalesce(s->>'model', '') <> '')::int AS llm_steps,
       count(*) FILTER (WHERE coalesce((s->>'cached')::boolean, false))::int AS cached_steps,
       count(*) FILTER (WHERE s->>'name' = 'self_check')::int AS self_checks,
       count(*) FILTER (WHERE s->>'name' = 'self_check' AND s->>'status' = 'ok'
         AND coalesce((s->'detail'->>'passed')::boolean, false))::int AS self_check_passed,
       count(*) FILTER (WHERE s->>'name' = 'revise')::int AS revisions,
       coalesce(sum(CASE WHEN jsonb_typeof(s->'detail'->'validation_notes') = 'array'
         THEN jsonb_array_length(s->'detail'->'validation_notes') ELSE 0 END), 0)::int AS validator_corrections
FROM step_rows;

-- Grounding и наполненность сохранённых полных анализов.
-- name: UserQualityAnalysisStats :one
WITH fulls AS (
  SELECT pa.* FROM personal_analyses pa
  WHERE pa.user_id = @user_id AND pa.depth = 'full' AND pa.method = 'llm' AND pa.created_at >= @since
)
SELECT count(*)::int AS total,
       count(*) FILTER (WHERE jsonb_array_length(coalesce(result->'insight'->'bank_view', '[]'::jsonb)) > 0)::int AS with_bank_view,
       count(*) FILTER (WHERE EXISTS (
         SELECT 1 FROM jsonb_array_elements(coalesce(result->'insight'->'evidence_sources', '[]'::jsonb)) e
         WHERE e->>'type' = 'company_data'
       ))::int AS with_company_facts,
       count(*) FILTER (WHERE EXISTS (
         SELECT 1 FROM jsonb_array_elements(coalesce(result->'insight'->'validation_notes', '[]'::jsonb)) n
         WHERE n #>> '{}' ILIKE '%доработан после самопроверки%'
       ))::int AS revised,
       coalesce(round(avg(jsonb_array_length(coalesce(result->'insight'->'evidence_sources', '[]'::jsonb)))), 0)::numeric(10,2) AS avg_sources,
       coalesce(round(avg(jsonb_array_length(coalesce(result->'insight'->'risks', '[]'::jsonb))), 2), 0)::numeric(10,2) AS avg_risks,
       coalesce(round(avg(jsonb_array_length(coalesce(result->'insight'->'opportunities', '[]'::jsonb))), 2), 0)::numeric(10,2) AS avg_opportunities,
       coalesce(round(avg(jsonb_array_length(coalesce(result->'insight'->'recommended_actions', '[]'::jsonb))), 2), 0)::numeric(10,2) AS avg_actions,
       coalesce(round(avg(jsonb_array_length(coalesce(result->'insight'->'validation_notes', '[]'::jsonb))), 2), 0)::numeric(10,2) AS avg_validation_notes,
       -- Фактологическая точность (анализы, выполненные после появления проверки).
       count(*) FILTER (WHERE result->'insight' ? 'source_coverage')::int AS with_coverage,
       coalesce(round(avg((result->'insight'->'source_coverage'->>'ratio')::numeric)
         FILTER (WHERE result->'insight' ? 'source_coverage') * 100), 0)::int AS avg_source_coverage,
       coalesce(sum((result->'insight'->'fact_check'->>'checked')::int), 0)::int AS checked_values,
       coalesce(sum(jsonb_array_length(coalesce(result->'insight'->'fact_check'->'unverified', '[]'::jsonb))), 0)::int AS unverified_values,
       count(*) FILTER (WHERE jsonb_array_length(coalesce(result->'insight'->'fact_check'->'unverified', '[]'::jsonb)) > 0)::int AS with_unverified,
       count(*) FILTER (WHERE result->'insight'->'source_completeness'->>'level' = 'full_text')::int AS level_full_text,
       count(*) FILTER (WHERE result->'insight'->'source_completeness'->>'level' = 'summary')::int AS level_summary,
       count(*) FILTER (WHERE result->'insight'->'source_completeness'->>'level' = 'title_only')::int AS level_title_only,
       -- Содержательность: механизм через признаки компании и конкретные действия.
       count(*) FILTER (WHERE result->'insight' ? 'used_company_factors')::int AS with_factor_field,
       count(*) FILTER (WHERE jsonb_array_length(coalesce(result->'insight'->'used_company_factors', '[]'::jsonb)) > 0)::int AS with_company_factors,
       coalesce(round(avg(jsonb_array_length(coalesce(result->'insight'->'used_company_factors', '[]'::jsonb)))
         FILTER (WHERE result->'insight' ? 'used_company_factors'), 2), 0)::numeric(10,2) AS avg_company_factors,
       coalesce(sum((SELECT count(*) FROM jsonb_array_elements(coalesce(result->'insight'->'recommended_actions', '[]'::jsonb)) a
         WHERE coalesce(a->>'object', '') <> '' AND coalesce(a->>'owner', '') <> '')), 0)::int AS concrete_actions,
       coalesce(sum((SELECT count(*) FROM jsonb_array_elements(coalesce(result->'insight'->'validation_notes', '[]'::jsonb)) n
         WHERE n #>> '{}' ILIKE '%общее действие%')), 0)::int AS generic_actions_removed
FROM fulls;

-- name: UserQualityFeedbackStats :one
SELECT count(*)::int AS total,
       count(*) FILTER (WHERE rating = 1)::int AS positive,
       count(*) FILTER (WHERE rating = -1)::int AS negative,
       count(*) FILTER (WHERE comment <> '')::int AS with_comment
FROM feedback
WHERE user_id = @user_id AND created_at >= @since;

-- name: UserQualityDaily :many
SELECT date_trunc('day', started_at)::timestamptz AS day,
       count(*)::int AS runs,
       count(*) FILTER (WHERE status = 'succeeded')::int AS succeeded,
       count(*) FILTER (WHERE status = 'failed')::int AS failed,
       coalesce(sum(prompt_tokens + completion_tokens), 0)::bigint AS tokens,
       coalesce(round(avg(extract(epoch FROM (finished_at - started_at)) * 1000)
         FILTER (WHERE finished_at IS NOT NULL)), 0)::bigint AS avg_latency_ms
FROM analysis_runs
WHERE user_id = @user_id AND kind = 'analyze' AND started_at >= @since
GROUP BY 1 ORDER BY 1;

-- Токены конкретной модели берутся из трассы шага, а не из итога запуска.
-- name: UserQualityModels :many
WITH step_rows AS (
  SELECT s
  FROM analysis_runs r CROSS JOIN LATERAL jsonb_array_elements(r.steps) s
  WHERE r.user_id = @user_id AND r.kind = 'analyze' AND r.started_at >= @since
)
SELECT (s->>'model')::text AS model,
       count(*)::int AS calls,
       coalesce(sum(coalesce((s->>'tokens')::int, 0)), 0)::bigint AS tokens,
       count(*) FILTER (WHERE s->>'status' = 'error')::int AS errors,
       count(*) FILTER (WHERE coalesce((s->>'cached')::boolean, false))::int AS cached
FROM step_rows
WHERE coalesce(s->>'model', '') <> ''
GROUP BY 1 ORDER BY calls DESC, model;

-- name: UserQualityPromptVersions :many
SELECT coalesce(nullif(pa.result->>'prompt_version', ''), 'unknown')::text AS prompt_version,
       count(*)::int AS analyses,
       coalesce(round(avg(pa.relevance_score)), 0)::int AS avg_relevance,
       count(f.id) FILTER (WHERE f.rating = 1)::int AS positive,
       count(f.id) FILTER (WHERE f.rating = -1)::int AS negative
FROM personal_analyses pa
LEFT JOIN feedback f ON f.analysis_id = pa.id
WHERE pa.user_id = @user_id AND pa.depth = 'full' AND pa.method = 'llm' AND pa.created_at >= @since
GROUP BY 1 ORDER BY analyses DESC, prompt_version;

-- Уточняющие вопросы к анализам: полезность, опора на источники и расход.
-- name: UserQualityQuestionStats :one
SELECT count(*)::int AS total,
       count(*) FILTER (WHERE status = 'succeeded')::int AS succeeded,
       count(*) FILTER (WHERE status = 'failed')::int AS failed,
       count(DISTINCT analysis_id)::int AS analyses,
       count(*) FILTER (WHERE method = 'llm' AND NOT cached)::int AS llm_calls,
       count(*) FILTER (WHERE method = 'deterministic')::int AS deterministic,
       (count(*) FILTER (WHERE cached) + coalesce(sum(cache_hits), 0))::int AS cache_hits,
       count(*) FILTER (WHERE preset IS NOT NULL)::int AS quick,
       count(*) FILTER (WHERE status = 'succeeded' AND cardinality(source_ids) > 0)::int AS with_sources,
       coalesce(round(avg(cardinality(source_ids)) FILTER (WHERE status = 'succeeded'), 2), 0)::numeric(10,2) AS avg_sources,
       count(*) FILTER (WHERE status = 'succeeded' AND coalesce((answer->>'out_of_scope')::boolean, false))::int AS out_of_scope,
       count(*) FILTER (WHERE status = 'succeeded' AND coalesce(answer->>'not_in_sources', '') <> '')::int AS not_in_sources,
       coalesce(sum(CASE WHEN jsonb_typeof(answer->'validation_notes') = 'array'
         THEN jsonb_array_length(answer->'validation_notes') ELSE 0 END), 0)::int AS validator_corrections,
       coalesce(round(avg(duration_ms) FILTER (WHERE method = 'llm' AND NOT cached AND status = 'succeeded')), 0)::int AS avg_llm_latency_ms,
       coalesce(sum(tokens), 0)::bigint AS tokens
FROM analysis_questions
WHERE user_id = @user_id AND created_at >= @since;

-- name: UserQualityQuestionPresets :many
SELECT coalesce(preset, 'custom')::text AS preset, count(*)::int AS questions
FROM analysis_questions
WHERE user_id = @user_id AND created_at >= @since AND status = 'succeeded'
GROUP BY 1 ORDER BY questions DESC, preset;

-- ИИ-помощник поддержки: как часто ответ модели отклоняется проверкой.
-- name: UserQualitySupportStats :one
SELECT count(*)::int AS total,
       count(*) FILTER (WHERE method = 'kb')::int AS from_kb,
       count(*) FILTER (WHERE method = 'llm')::int AS llm_accepted,
       count(*) FILTER (WHERE method = 'fallback')::int AS llm_rejected,
       count(*) FILTER (WHERE cached)::int AS cached,
       count(*) FILTER (WHERE coalesce((answer->>'out_of_scope')::boolean, false))::int AS out_of_scope,
       count(*) FILTER (WHERE coalesce((answer->>'not_in_kb')::boolean, false))::int AS not_in_kb,
       coalesce(sum(jsonb_array_length(coalesce(answer->'checks'->'notes', '[]'::jsonb))), 0)::int AS check_corrections,
       coalesce(sum(tokens), 0)::bigint AS tokens
FROM support_messages
WHERE user_id = @user_id AND created_at >= @since AND status = 'succeeded';
