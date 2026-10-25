-- +goose Up
-- +goose StatementBegin
-- Для общих лент сохраняются только экономические рубрики источника
-- (поле <category> в RSS). Пустой список — сохраняются все материалы.
ALTER TABLE sources ADD COLUMN category_filter TEXT[] NOT NULL DEFAULT '{}';

UPDATE sources SET feed_url = 'https://www.interfax.ru/rss', category_filter = '{"Экономика"}' WHERE code = 'interfax';
UPDATE sources SET category_filter = '{"Экономика и бизнес"}' WHERE code = 'tass';

-- Происхождение темы статьи: 'llm' — классификация моделью,
-- 'keywords' — предварительная разметка по словарю (без ИИ),
-- 'metadata' — тема материала банка по его названию.
ALTER TABLE article_topics ADD COLUMN method TEXT NOT NULL DEFAULT 'llm'
  CHECK (method IN ('llm','keywords','metadata'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE article_topics DROP COLUMN method;
ALTER TABLE sources DROP COLUMN category_filter;
-- +goose StatementEnd
