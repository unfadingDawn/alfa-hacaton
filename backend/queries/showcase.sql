-- Публичная витрина лендинга: реальные анализы модели, без имён пользователей.
-- Предпочитаются новости, которые модель разобрала для нескольких разных ролей.
-- name: ShowcaseArticles :many
SELECT pa.article_id, count(DISTINCT pa.role_code)::int AS roles, max(pa.created_at)::timestamptz AS last_at
FROM personal_analyses pa
JOIN articles a ON a.id = pa.article_id
WHERE pa.method = 'llm' AND a.duplicate_of IS NULL AND length(pa.why_it_matters) > 0
GROUP BY pa.article_id
ORDER BY roles DESC, (bool_or(pa.depth = 'full')) DESC, last_at DESC
LIMIT 1;

-- name: ShowcaseAnalyses :many
SELECT DISTINCT ON (pa.role_code)
       pa.role_code, pa.depth, pa.relevance_score, pa.importance_level, pa.impact_type, pa.why_it_matters,
       coalesce(pa.result->'insight'->'bank_view'->0->>'text', '')::text AS bank_view,
       coalesce(pa.result->'insight'->'risks'->0->>'title', '')::text AS risk,
       coalesce(pa.result->'insight'->'opportunities'->0->>'title', '')::text AS opportunity,
       c.okved_name AS industry, coalesce(c.display_name, '')::text AS company_name
FROM personal_analyses pa
JOIN companies c ON c.id = pa.company_id
WHERE pa.article_id = @article_id AND pa.method = 'llm' AND length(pa.why_it_matters) > 0
ORDER BY pa.role_code, (pa.depth = 'full') DESC, pa.created_at DESC
LIMIT 3;

-- name: ShowcaseArticle :one
SELECT a.id, a.title, a.summary, a.url, a.published_at, s.name AS source_name, s.kind AS source_kind
FROM articles a JOIN sources s ON s.id = a.source_id
WHERE a.id = @id;

-- Живая лента: последние новости и материалы аналитиков (без персональных данных).
-- name: ShowcaseLatest :many
SELECT a.id, a.title, a.url, a.published_at, s.name AS source_name, s.kind AS source_kind
FROM articles a JOIN sources s ON s.id = a.source_id
WHERE a.duplicate_of IS NULL AND s.kind = @kind::text
  AND EXISTS (SELECT 1 FROM article_topics t WHERE t.article_id = a.id) -- только деловая повестка
ORDER BY a.published_at DESC
LIMIT @lim;
