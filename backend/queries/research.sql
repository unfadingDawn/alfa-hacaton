-- name: UpsertResearchDocument :one
INSERT INTO research_documents (file_name, title, subtitle, published_on, author_unit, pages, is_outdated, lead_text)
VALUES (@file_name, @title, @subtitle, sqlc.narg('published_on'), @author_unit, @pages, @is_outdated, @lead_text)
ON CONFLICT (file_name) DO UPDATE SET
  title = EXCLUDED.title, subtitle = EXCLUDED.subtitle, published_on = EXCLUDED.published_on,
  author_unit = EXCLUDED.author_unit, pages = EXCLUDED.pages, is_outdated = EXCLUDED.is_outdated,
  lead_text = EXCLUDED.lead_text, imported_at = now()
RETURNING id;

-- name: DeleteResearchChunks :exec
DELETE FROM research_chunks WHERE document_id = @document_id;

-- name: InsertResearchChunk :exec
INSERT INTO research_chunks (document_id, chunk_no, page, content)
VALUES (@document_id, @chunk_no, @page, @content);

-- name: ListResearchDocuments :many
SELECT id, file_name, title, subtitle, published_on, author_unit, pages, is_outdated, lead_text
FROM research_documents
ORDER BY published_on DESC NULLS LAST;

-- name: GetResearchDocument :one
SELECT * FROM research_documents WHERE id = @id;

-- name: SearchResearchChunks :many
SELECT c.id, c.document_id, c.page, c.content, d.title, d.published_on, d.is_outdated,
       ts_rank(c.tsv, to_tsquery('russian', @tsquery))::real AS rank
FROM research_chunks c JOIN research_documents d ON d.id = c.document_id
WHERE c.tsv @@ to_tsquery('russian', @tsquery)
ORDER BY d.is_outdated, rank DESC
LIMIT @lim;

-- name: GetResearchChunks :many
SELECT c.id, c.document_id, c.page, c.content, d.title, d.published_on, d.is_outdated
FROM research_chunks c JOIN research_documents d ON d.id = c.document_id
WHERE c.id = ANY(@ids::int[]);

-- name: GetResearchArticleID :one
SELECT a.id FROM articles a JOIN sources s ON s.id = a.source_id
WHERE s.code = 'alfa_research' AND a.guid = @guid;

-- name: UpsertResearchArticle :one
INSERT INTO articles (source_id, url, guid, title, summary, body, published_at, title_norm, content_hash)
SELECT s.id, @url, @guid, @title, @summary, @body, @published_at, @title_norm, @content_hash
FROM sources s WHERE s.code = 'alfa_research'
ON CONFLICT (url) DO UPDATE SET
  title = EXCLUDED.title, summary = EXCLUDED.summary, body = EXCLUDED.body,
  published_at = EXCLUDED.published_at, title_norm = EXCLUDED.title_norm, content_hash = EXCLUDED.content_hash
RETURNING id;

-- name: ListResearchChunksByDoc :many
SELECT id, chunk_no, page, content FROM research_chunks WHERE document_id = @document_id ORDER BY chunk_no;
