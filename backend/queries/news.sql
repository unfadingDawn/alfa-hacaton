-- name: ListSources :many
SELECT * FROM sources ORDER BY id;

-- name: ListEnabledFeeds :many
SELECT * FROM sources WHERE kind = 'rss' AND enabled AND feed_url IS NOT NULL ORDER BY id;

-- name: GetSourceByCode :one
SELECT * FROM sources WHERE code = @code;

-- name: UpdateSourceFetchStatus :exec
UPDATE sources SET last_fetch_at = now(), last_fetch_status = @status WHERE id = @id;

-- name: FindDuplicateArticle :one
SELECT id FROM articles
WHERE (content_hash = @content_hash OR title_norm = @title_norm)
  AND duplicate_of IS NULL
  AND published_at > @since
ORDER BY published_at
LIMIT 1;

-- name: InsertArticle :one
INSERT INTO articles (source_id, url, guid, title, summary, body, published_at,
                      title_norm, content_hash, duplicate_of, submitted_by)
VALUES (@source_id, @url, @guid, @title, @summary, @body, @published_at,
        @title_norm, @content_hash, sqlc.narg('duplicate_of'), sqlc.narg('submitted_by'))
ON CONFLICT (url) DO NOTHING
RETURNING id;

-- Публикация обновлена источником по тому же URL: обновляем содержание и
-- сбрасываем классификацию, чтобы анализ строился по актуальному тексту.
-- name: UpdateArticleContent :one
UPDATE articles SET title = @title, summary = @summary, body = @body, title_norm = @title_norm,
  content_hash = @content_hash, updated_at = now(), classification = NULL, classified_at = NULL
WHERE url = @url AND submitted_by IS NULL AND content_hash <> @content_hash
RETURNING id;

-- Кандидаты для поиска одного события из разных источников (заголовки за окно).
-- name: ListDedupCandidates :many
SELECT id, title, summary, source_id FROM articles
WHERE duplicate_of IS NULL AND submitted_by IS NULL
  AND published_at BETWEEN @date_from AND @date_to
ORDER BY published_at
LIMIT 500;

-- name: GetArticle :one
SELECT a.id, a.source_id, a.url, a.guid, a.title, a.summary, a.body, a.published_at, a.fetched_at,
       a.duplicate_of, a.submitted_by, a.classified_at, a.classification, a.content_level, a.updated_at, a.content_hash,
       s.code AS source_code, s.name AS source_name, s.kind AS source_kind, s.full_text_policy
FROM articles a JOIN sources s ON s.id = a.source_id
WHERE a.id = @id;

-- name: ListUnclassifiedArticles :many
SELECT a.id, a.title, a.summary, a.body, a.published_at, s.name AS source_name
FROM articles a JOIN sources s ON s.id = a.source_id
WHERE a.classified_at IS NULL AND a.duplicate_of IS NULL
ORDER BY a.published_at DESC
LIMIT @lim;

-- name: SetArticleClassification :exec
UPDATE articles SET classification = @classification, classified_at = now() WHERE id = @id;

-- name: DeleteArticleTopics :exec
DELETE FROM article_topics WHERE article_id = @article_id;

-- name: AddArticleTopic :exec
INSERT INTO article_topics (article_id, topic_code, confidence, method)
VALUES (@article_id, @topic_code, @confidence, @method)
ON CONFLICT (article_id, topic_code) DO UPDATE SET confidence = EXCLUDED.confidence, method = EXCLUDED.method;

-- name: DeleteArticleTopicsByMethod :exec
DELETE FROM article_topics WHERE article_id = @article_id AND method = @method;

-- Статьи без тем — для разметки по словарю (без ИИ).
-- name: ListArticlesWithoutTopics :many
SELECT a.id, a.title, a.summary, a.body FROM articles a
WHERE a.duplicate_of IS NULL
  AND NOT EXISTS (SELECT 1 FROM article_topics t WHERE t.article_id = a.id)
LIMIT @lim;

-- name: ListTopicsForArticles :many
SELECT at.article_id, at.topic_code, t.name, at.method
FROM article_topics at JOIN topics t ON t.code = at.topic_code
WHERE at.article_id = ANY(@ids::uuid[])
ORDER BY at.confidence DESC;

-- Кандидаты для персонального ранжирования: свежие классифицированные
-- статьи, для которых у пользователя ещё нет оценки.
-- name: ListRankingCandidates :many
SELECT a.id, a.title, a.summary, a.published_at, a.classification, s.name AS source_name, s.kind AS source_kind
FROM articles a
JOIN sources s ON s.id = a.source_id
WHERE a.duplicate_of IS NULL
  AND a.classified_at IS NOT NULL
  AND a.published_at > @since
  AND NOT EXISTS (
    SELECT 1 FROM personal_analyses pa
    WHERE pa.article_id = a.id AND pa.user_id = @user_id AND pa.method = 'llm'
  )
ORDER BY a.published_at DESC
LIMIT @lim;

-- name: ListFeed :many
SELECT a.id, a.url, a.title, a.summary, a.published_at,
       s.code AS source_code, s.name AS source_name, s.kind AS source_kind,
       (pa.id IS NOT NULL)::boolean AS has_analysis,
       coalesce(pa.depth, '')::text AS depth, coalesce(pa.method, '')::text AS method,
       coalesce(pa.relevance_score, 0)::smallint AS relevance_score,
       coalesce(pa.importance_level, '')::text AS importance_level,
       coalesce(pa.impact_type, '')::text AS impact_type,
       coalesce(pa.matched_topics, '{}')::text[] AS matched_topics,
       coalesce(pa.why_it_matters, '')::text AS why_it_matters,
       (sa.user_id IS NOT NULL)::boolean AS is_saved
FROM articles a
JOIN sources s ON s.id = a.source_id
LEFT JOIN LATERAL (
  SELECT * FROM personal_analyses x
  WHERE x.article_id = a.id AND x.user_id = @user_id
  ORDER BY (x.depth = 'full') DESC, (x.method = 'llm') DESC
  LIMIT 1
) pa ON true
LEFT JOIN saved_articles sa ON sa.article_id = a.id AND sa.user_id = @user_id
WHERE a.duplicate_of IS NULL
  AND (sqlc.narg('q')::text IS NULL OR a.tsv @@ websearch_to_tsquery('russian', sqlc.narg('q')::text))
  AND (sqlc.narg('topic')::text IS NULL OR EXISTS (
        SELECT 1 FROM article_topics t WHERE t.article_id = a.id AND t.topic_code = sqlc.narg('topic')::text))
  AND (sqlc.narg('min_importance')::int IS NULL OR
        (CASE pa.importance_level WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END)
          >= sqlc.narg('min_importance')::int)
  AND (sqlc.narg('impact')::text IS NULL OR pa.impact_type = sqlc.narg('impact')::text)
  AND (sqlc.narg('since')::timestamptz IS NULL OR a.published_at >= sqlc.narg('since')::timestamptz)
  AND (sqlc.narg('source_kind')::text IS NULL OR s.kind = sqlc.narg('source_kind')::text)
  AND (NOT @saved_only::boolean OR sa.user_id IS NOT NULL)
ORDER BY
  CASE WHEN @sort::text = 'relevance' THEN coalesce(pa.relevance_score, -1) END DESC NULLS LAST,
  CASE WHEN @sort::text = 'importance' THEN
    (CASE pa.importance_level WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END) END DESC NULLS LAST,
  a.published_at DESC
LIMIT @lim OFFSET @off;

-- name: CountFeed :one
SELECT count(*)::int
FROM articles a
JOIN sources s ON s.id = a.source_id
LEFT JOIN LATERAL (
  SELECT * FROM personal_analyses x
  WHERE x.article_id = a.id AND x.user_id = @user_id
  ORDER BY (x.depth = 'full') DESC, (x.method = 'llm') DESC
  LIMIT 1
) pa ON true
LEFT JOIN saved_articles sa ON sa.article_id = a.id AND sa.user_id = @user_id
WHERE a.duplicate_of IS NULL
  AND (sqlc.narg('q')::text IS NULL OR a.tsv @@ websearch_to_tsquery('russian', sqlc.narg('q')::text))
  AND (sqlc.narg('topic')::text IS NULL OR EXISTS (
        SELECT 1 FROM article_topics t WHERE t.article_id = a.id AND t.topic_code = sqlc.narg('topic')::text))
  AND (sqlc.narg('min_importance')::int IS NULL OR
        (CASE pa.importance_level WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END)
          >= sqlc.narg('min_importance')::int)
  AND (sqlc.narg('impact')::text IS NULL OR pa.impact_type = sqlc.narg('impact')::text)
  AND (sqlc.narg('since')::timestamptz IS NULL OR a.published_at >= sqlc.narg('since')::timestamptz)
  AND (sqlc.narg('source_kind')::text IS NULL OR s.kind = sqlc.narg('source_kind')::text)
  AND (NOT @saved_only::boolean OR sa.user_id IS NOT NULL);

-- name: SearchRelatedArticles :many
SELECT a.id, a.title, a.summary, a.url, a.published_at, s.name AS source_name,
       ts_rank(a.tsv, to_tsquery('russian', @tsquery))::real AS rank
FROM articles a JOIN sources s ON s.id = a.source_id
WHERE a.duplicate_of IS NULL
  AND s.kind <> 'alfa_research' -- материалы банка ищутся отдельным инструментом по фрагментам
  AND a.id <> @exclude_id
  AND a.tsv @@ to_tsquery('russian', @tsquery)
ORDER BY rank DESC, a.published_at DESC
LIMIT @lim;

-- name: SaveArticle :exec
INSERT INTO saved_articles (user_id, article_id, note)
VALUES (@user_id, @article_id, @note)
ON CONFLICT (user_id, article_id) DO UPDATE SET note = EXCLUDED.note;

-- name: UnsaveArticle :exec
DELETE FROM saved_articles WHERE user_id = @user_id AND article_id = @article_id;

-- name: IsArticleSaved :one
SELECT EXISTS (SELECT 1 FROM saved_articles WHERE user_id = @user_id AND article_id = @article_id) AS saved;

-- name: ArticleStats :one
SELECT count(*)::int AS total,
       count(*) FILTER (WHERE classified_at IS NOT NULL)::int AS classified,
       max(published_at)::timestamptz AS latest
FROM articles WHERE duplicate_of IS NULL;

-- Статьи для локальной предварительной оценки (без LLM) для пользователя.
-- name: ListArticlesForRules :many
SELECT a.id, a.title, a.summary, a.published_at,
       coalesce(array_agg(at.topic_code) FILTER (WHERE at.topic_code IS NOT NULL), '{}')::text[] AS topics,
       (min(s.kind) = 'alfa_research')::boolean AS is_research
FROM articles a
JOIN sources s ON s.id = a.source_id
LEFT JOIN article_topics at ON at.article_id = a.id
WHERE a.duplicate_of IS NULL
  AND a.published_at > @since
  AND NOT EXISTS (
    SELECT 1 FROM personal_analyses pa
    WHERE pa.article_id = a.id AND pa.user_id = @user_id
      AND (pa.method = 'llm' OR pa.depth = 'full' OR pa.input_hash = @input_hash)
  )
GROUP BY a.id
ORDER BY a.published_at DESC
LIMIT @lim;

-- Последние новости для обучения локальной TF-IDF + LSA модели.
-- name: ListSemanticCorpus :many
SELECT a.id, a.title, a.summary
FROM articles a
WHERE a.duplicate_of IS NULL
ORDER BY a.published_at DESC
LIMIT @lim;

-- Сообщения Банка России (официальный источник) для блока «рынок».
-- name: ListMarketNews :many
SELECT a.id, a.url, a.title, a.summary, a.published_at,
       s.code AS source_code, s.name AS source_name, s.kind AS source_kind,
       (pa.id IS NOT NULL)::boolean AS has_analysis,
       coalesce(pa.depth, '')::text AS depth, coalesce(pa.method, '')::text AS method,
       coalesce(pa.relevance_score, 0)::smallint AS relevance_score,
       coalesce(pa.importance_level, '')::text AS importance_level,
       coalesce(pa.impact_type, '')::text AS impact_type,
       coalesce(pa.matched_topics, '{}')::text[] AS matched_topics,
       coalesce(pa.why_it_matters, '')::text AS why_it_matters,
       (sa.user_id IS NOT NULL)::boolean AS is_saved
FROM articles a
JOIN sources s ON s.id = a.source_id
LEFT JOIN LATERAL (
  SELECT * FROM personal_analyses x
  WHERE x.article_id = a.id AND x.user_id = @user_id
  ORDER BY (x.depth = 'full') DESC, (x.method = 'llm') DESC
  LIMIT 1
) pa ON true
LEFT JOIN saved_articles sa ON sa.article_id = a.id AND sa.user_id = @user_id
WHERE a.duplicate_of IS NULL AND s.code LIKE 'cbr%'
ORDER BY a.published_at DESC
LIMIT @lim;
