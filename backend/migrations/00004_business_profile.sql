-- +goose Up
-- +goose StatementBegin

-- Фото профиля (data URL, уменьшенное на клиенте; размер проверяется в API).
ALTER TABLE users ADD COLUMN avatar TEXT;

-- Откуда получены сведения о компании из личного кабинета Альфа-Бизнеса:
-- 'user' — введены пользователем при регистрации,
-- 'alfa_business' — получены при входе из Альфа-Бизнеса (будущая интеграция).
-- Сами сведения хранятся в companies.profile->'business'.

-- По ответам организаторов: крупный бизнес определяется по выручке группы
-- компаний, а REVENUE_COMPANY = 1000 — техническая заглушка (как NULL).
ALTER TABLE companies ADD COLUMN group_revenue NUMERIC(20,2);
ALTER TABLE companies ADD COLUMN group_size INTEGER;
CREATE INDEX companies_group_revenue_idx ON companies (group_revenue DESC);

UPDATE companies SET revenue = NULL WHERE revenue = 1000;
UPDATE companies c SET group_revenue = g.rev, group_size = g.n
FROM (SELECT group_id, sum(revenue) AS rev, count(*)::int AS n FROM companies WHERE group_id IS NOT NULL GROUP BY group_id) g
WHERE c.group_id = g.group_id;
UPDATE companies SET group_revenue = revenue, group_size = 1 WHERE group_id IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS companies_group_revenue_idx;
ALTER TABLE companies DROP COLUMN group_size;
ALTER TABLE companies DROP COLUMN group_revenue;
ALTER TABLE users DROP COLUMN avatar;
-- +goose StatementEnd
