# Holdout evaluation AlfaInsight

Набор предназначен для воспроизводимой проверки ранжирования и структурированного персонального анализа. Все компании, новости и источники синтетические. Тексты не копируют новости, использованные при разработке промптов.

## Состав

- `companies.json` — 6 компаний разных отраслей, включая профиль с неполными данными;
- `articles.json` — 25 новых публикаций, включая шум и межисточниковый дубль;
- `expected.json` — 6 запросов для 4 ролей и экспертные свойства релевантных/контрольных пар.

Отсутствующая в `judgments` пара считается нерелевантной для метрик ранжирования. Полный эталонный текст анализа намеренно не хранится: проверяются диапазон score, темы, факторы компании, тип влияния, источники и запрещённые неподтверждённые утверждения.

## Offline/mock

Не использует API-ключ:

```bash
cd backend
go run ./cmd/eval -data ../data/evaluation -mode mock -out ../evaluation-results/mock.json
```

Mock — детерминированный локальный baseline, а не оценка качества LLM. Он проверяет загрузку holdout, метрики, ранжирование, дедупликацию, сохранение отчёта и CI-контур.

## Live

Live-режим запускается только вручную и расходует общий бюджет:

```bash
set -a; . ./.env; set +a
cd backend
go run ./cmd/eval \
  -data ../data/evaluation \
  -mode live \
  -confirm-live \
  -max-live-cases 5 \
  -out ../evaluation-results/live.json
```

По умолчанию выполняются не более 5 кейсов. `-max-live-cases 0` запускает все положительные judgments. Стоимость рассчитывается только при известных тарифах:

```bash
-input-usd-per-million 0.0 -output-usd-per-million 0.0
```

или через `EVAL_INPUT_USD_PER_MILLION` и `EVAL_OUTPUT_USD_PER_MILLION`.

## Сравнение версий

```bash
go run ./cmd/eval \
  -data ../data/evaluation \
  -mode mock \
  -baseline ../evaluation-results/previous.json \
  -out ../evaluation-results/current.json
```

Отчёт содержит версию набора, `PromptVersion`, модель, timestamp, результаты каждого кейса и агрегированные метрики.
