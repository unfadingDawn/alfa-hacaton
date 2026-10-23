-- name: GetCompany :one
SELECT * FROM companies WHERE id = @id;

-- name: SearchCompanies :many
SELECT id, display_name, okved_code, okved_name, region, city, revenue,
       client_type, eco_stage, workers_fns, group_id, group_revenue, group_size
FROM companies
WHERE (sqlc.narg('q')::text IS NULL
         OR id = sqlc.narg('q')::text
         OR okved_name ILIKE '%' || sqlc.narg('q')::text || '%'
         OR okved_code LIKE sqlc.narg('q')::text || '%'
         OR display_name ILIKE '%' || sqlc.narg('q')::text || '%')
  AND (sqlc.narg('region')::text IS NULL OR region = sqlc.narg('region')::text)
  -- Сегмент крупного бизнеса определяется по выручке группы компаний (ответ организаторов).
  AND (sqlc.narg('min_revenue')::numeric IS NULL OR group_revenue >= sqlc.narg('min_revenue')::numeric)
  AND (NOT @clients_only::boolean OR client_type = 'CLIENT')
ORDER BY revenue DESC NULLS LAST, group_revenue DESC NULLS LAST, id
LIMIT @lim OFFSET @off;

-- name: ListCompanyRegions :many
SELECT region, count(*)::int AS companies
FROM companies
WHERE region IS NOT NULL
GROUP BY region
ORDER BY companies DESC;

-- name: UpdateCompanyProfile :one
UPDATE companies
SET display_name = @display_name,
    profile = @profile,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: CompanyPaymentTotals :one
SELECT
  coalesce(sum(amount) FILTER (WHERE payer = @company_id AND NOT is_internal), 0)::numeric       AS outgoing_amount,
  coalesce(sum(pay_count) FILTER (WHERE payer = @company_id AND NOT is_internal), 0)::bigint      AS outgoing_count,
  count(DISTINCT beneficiary) FILTER (WHERE payer = @company_id AND NOT is_internal)::int          AS supplier_count,
  coalesce(sum(amount) FILTER (WHERE beneficiary = @company_id AND NOT is_internal), 0)::numeric AS incoming_amount,
  coalesce(sum(pay_count) FILTER (WHERE beneficiary = @company_id AND NOT is_internal), 0)::bigint AS incoming_count,
  count(DISTINCT payer) FILTER (WHERE beneficiary = @company_id AND NOT is_internal)::int          AS customer_count
FROM payments
WHERE payer = @company_id OR beneficiary = @company_id;

-- Компании, которым платит выбранная компания (поставщики/подрядчики).
-- name: TopPaymentRecipients :many
SELECT p.beneficiary AS counterparty_id,
       sum(p.amount)::numeric      AS amount,
       sum(p.pay_count)::bigint    AS pay_count,
       c.okved_code, c.okved_name, c.okved_division, c.region
FROM payments p
LEFT JOIN companies c ON c.id = p.beneficiary
WHERE p.payer = @company_id AND NOT p.is_internal
GROUP BY p.beneficiary, c.okved_code, c.okved_name, c.okved_division, c.region
ORDER BY amount DESC
LIMIT @lim;

-- Компании, которые платят выбранной компании (покупатели/заказчики).
-- name: TopPaymentSenders :many
SELECT p.payer AS counterparty_id,
       sum(p.amount)::numeric      AS amount,
       sum(p.pay_count)::bigint    AS pay_count,
       c.okved_code, c.okved_name, c.okved_division, c.region
FROM payments p
LEFT JOIN companies c ON c.id = p.payer
WHERE p.beneficiary = @company_id AND NOT p.is_internal
GROUP BY p.payer, c.okved_code, c.okved_name, c.okved_division, c.region
ORDER BY amount DESC
LIMIT @lim;

-- Отрасли контрагентов по объёму платежей (только контрагенты, найденные в справочнике).
-- name: CounterpartyIndustries :many
SELECT direction, okved_division, min(okved_name)::text AS sample_okved_name,
       count(*)::int AS counterparties, sum(amount)::numeric AS amount
FROM (
  SELECT 'outgoing'::text AS direction, c.okved_division, c.okved_name, p.beneficiary AS cp, sum(p.amount) AS amount
  FROM payments p JOIN companies c ON c.id = p.beneficiary
  WHERE p.payer = @company_id AND NOT p.is_internal
  GROUP BY c.okved_division, c.okved_name, p.beneficiary
  UNION ALL
  SELECT 'incoming'::text, c.okved_division, c.okved_name, p.payer, sum(p.amount)
  FROM payments p JOIN companies c ON c.id = p.payer
  WHERE p.beneficiary = @company_id AND NOT p.is_internal
  GROUP BY c.okved_division, c.okved_name, p.payer
) t
GROUP BY direction, okved_division
ORDER BY direction, amount DESC;

-- name: GroupMembers :many
SELECT id, display_name, okved_code, okved_name, region, revenue
FROM companies
WHERE group_id = @group_id AND id <> @company_id
ORDER BY revenue DESC NULLS LAST
LIMIT @lim;

-- name: CountGroupMembers :one
SELECT count(*)::int FROM companies WHERE group_id = @group_id;

-- name: LinkedPersonsSummary :one
SELECT count(DISTINCT person_id)::int AS persons,
       coalesce(array_agg(DISTINCT region_name) FILTER (WHERE region_name IS NOT NULL), '{}')::text[] AS regions
FROM person_links
WHERE company_id = @company_id;

-- Компании, связанные с выбранной через общих (обезличенных) физлиц.
-- name: CompaniesSharingPersons :many
SELECT other.company_id,
       count(DISTINCT other.person_id)::int AS shared_persons,
       c.okved_code, c.okved_name, c.region
FROM person_links mine
JOIN person_links other ON other.person_id = mine.person_id AND other.company_id <> mine.company_id
LEFT JOIN companies c ON c.id = other.company_id
WHERE mine.company_id = @company_id
GROUP BY other.company_id, c.okved_code, c.okved_name, c.region
ORDER BY shared_persons DESC
LIMIT @lim;

-- name: CountImportIssues :many
SELECT file_name, kind, count(*)::int AS issues
FROM import_issues
GROUP BY file_name, kind
ORDER BY file_name, kind;

-- name: DatasetStats :one
SELECT
  (SELECT count(*) FROM companies)::int     AS companies,
  (SELECT count(*) FROM companies WHERE group_revenue >= 15000000000)::int AS large_companies,
  (SELECT count(*) FROM person_links)::int  AS person_links,
  (SELECT count(*) FROM payments)::int      AS payments;

-- name: DeleteImportIssues :exec
DELETE FROM import_issues WHERE file_name = @file_name;

-- name: AddImportIssue :exec
INSERT INTO import_issues (file_name, source_row, kind, detail)
VALUES (@file_name, @source_row, @kind, @detail);
