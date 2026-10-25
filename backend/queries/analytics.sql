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
