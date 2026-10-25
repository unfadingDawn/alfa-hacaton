-- +goose Up
-- +goose StatementBegin

-- Полнота текста источника. Вычисляется самой БД, поэтому одинакова для RSS,
-- материалов банка и новостей, добавленных вручную:
--   full_text  — есть полный текст (материал банка, добавлено вручную, лента
--                с разрешённым полным текстом);
--   summary    — заголовок и содержательный анонс;
--   title_only — только заголовок (анонса нет или он повторяет заголовок).
ALTER TABLE articles ADD COLUMN content_level TEXT GENERATED ALWAYS AS (
  CASE
    WHEN length(btrim(body)) >= 200 THEN 'full_text'
    WHEN length(btrim(summary)) >= 60
         AND position(lower(left(btrim(title), 60)) IN lower(summary)) <> 1 THEN 'summary'
    ELSE 'title_only'
  END) STORED;
CREATE INDEX articles_content_level_idx ON articles (content_level);

-- Когда публикация обновлена источником по тому же URL.
ALTER TABLE articles ADD COLUMN updated_at TIMESTAMPTZ;

-- Политика полного текста источника:
--   none — хранится только заголовок, анонс и ссылка (по умолчанию для СМИ);
--   feed — разрешено сохранять полный текст, если источник сам передаёт его
--          в RSS (content:encoded, yandex:full-text). Сайты не скачиваются,
--          защиты и платный доступ не обходятся.
ALTER TABLE sources ADD COLUMN full_text_policy TEXT NOT NULL DEFAULT 'none'
  CHECK (full_text_policy IN ('none', 'feed'));
-- Официальные публикации регулятора: полный текст, если он есть в ленте.
UPDATE sources SET full_text_policy = 'feed' WHERE code IN ('cbr_press', 'cbr_events');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sources DROP COLUMN full_text_policy;
ALTER TABLE articles DROP COLUMN updated_at;
DROP INDEX IF EXISTS articles_content_level_idx;
ALTER TABLE articles DROP COLUMN content_level;
-- +goose StatementEnd
