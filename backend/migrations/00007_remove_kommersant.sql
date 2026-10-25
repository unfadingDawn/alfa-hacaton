-- +goose Up
-- +goose StatementBegin
DELETE FROM sources WHERE code = 'kommersant';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
INSERT INTO sources (code, name, kind, feed_url, homepage, terms_note, enabled, category_filter) VALUES
 ('kommersant', 'Коммерсантъ — Экономика', 'rss', 'https://www.kommersant.ru/rss/section-economics.xml', 'https://www.kommersant.ru',
  'Официальная публичная RSS-лента раздела «Экономика»; хранится только анонс и ссылка', true, '{}')
ON CONFLICT (code) DO NOTHING;
-- +goose StatementEnd
