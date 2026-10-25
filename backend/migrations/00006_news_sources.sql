-- +goose Up
-- +goose StatementBegin

-- Официальные публичные RSS-ленты. Для общих лент деловых изданий
-- category_filter ограничивает поток рубриками, полезными для бизнес-аналитики.
INSERT INTO sources (code, name, kind, feed_url, homepage, terms_note, enabled, category_filter) VALUES
 ('vedomosti', 'Ведомости', 'rss', 'https://www.vedomosti.ru/rss/news', 'https://www.vedomosti.ru',
  'Официальная публичная RSS-лента; хранится только анонс и ссылка', true,
  ARRAY['Бизнес','Экономика','Финансы','Инвестиции','Технологии','Авто']),
 ('finmarket', 'Финмаркет', 'rss', 'https://www.finmarket.ru/rss/mainnews.asp', 'https://www.finmarket.ru',
  'Официальная публичная RSS-лента в Windows-1251; хранится только анонс и ссылка', true, '{}'),
 ('moex', 'Московская биржа', 'rss', 'https://www.moex.com/export/news.aspx?cat=101', 'https://www.moex.com',
  'Официальная RSS-лента главных новостей Московской биржи', true, '{}')
ON CONFLICT (code) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM sources WHERE code IN ('vedomosti', 'finmarket', 'moex');
-- +goose StatementEnd
