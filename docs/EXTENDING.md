# Как расширять AlfaInsight

Здесь собраны пошаговые инструкции для типовых изменений: новый источник, отрасль, тип клиента, роль, тема, материал банка, промпт, валидатор и быстрый вопрос. После любого изменения выполните проверку по [OPERATIONS.md](OPERATIONS.md#проверка-изменений-перед-релизом).

Общие правила:

- схема БД меняется только новой goose-миграцией `backend/migrations/000NN_*.sql` с разделами `Up` и `Down`. Уже применённые миграции не редактируются. Интеграционный тест проверяет, что все миграции откатываются;
- SQL-запросы лежат в `backend/queries/*.sql`, Go-код к ним генерируется командой `sqlc generate` (sqlc 1.30) в каталоге `backend`. Сгенерированный код `internal/db/dbq` коммитится;
- изменения API отражаются в `backend/api/openapi.yaml` и `frontend/src/lib/types.ts`;
- справочники тем и ролей кэшируются в памяти API, поэтому после их изменения API нужно перезапустить.

## Источник новостей

### RSS-лента

1. Добавьте миграцию:

   ```sql
   -- +goose Up
   INSERT INTO sources (code, name, kind, feed_url, homepage, terms_note, enabled, category_filter, full_text_policy)
   VALUES ('example_econ', 'Пример: деловое издание', 'rss', 'https://example.ru/rss/economy.xml', 'https://example.ru',
           'Официальная публичная RSS-лента; хранится только анонс и ссылка', true, ARRAY['Экономика'], 'none')
   ON CONFLICT (code) DO NOTHING;
   -- +goose Down
   DELETE FROM sources WHERE code = 'example_econ';
   ```

2. Поля:
   - `category_filter` — рубрики `<category>` из ленты, которые нужно сохранять. Пустой массив сохраняет всё. Вложенные рубрики («Экономика / Налоги») совпадают с родительской;
   - `full_text_policy`: `none` — только заголовок, анонс и ссылка (по умолчанию для СМИ); `feed` — можно хранить полный текст, если лента сама передаёт его (`content:encoded`, `yandex:full-text`, Atom `content`). Ставьте `feed` только когда условия источника это разрешают. Сайты не скачиваются, защиты и платный доступ не обходятся;
   - `terms_note` — условия использования, показываются в справочнике источников.
3. Проверьте ленту без записи в БД: добавьте её URL в список лент в `backend/internal/news/rss_live_smoke_test.go` и запустите `go test -tags=live ./internal/news -run '^TestLiveRSSSmoke$'`.
4. Перезапустите API: лента подхватится при следующем опросе (`NEWS_FETCH_INTERVAL`), либо запустите `importer -news`.

Нормализация, вычисление `content_level`, дедупликация (в том числе между изданиями) и обновление публикаций по тому же URL работают для нового источника автоматически.

### Источник другого типа (API, Telegram, e-mail-рассылка)

1. Расширьте ограничение `sources.kind` миграцией (`ALTER TABLE sources DROP CONSTRAINT ...; ADD CONSTRAINT ... CHECK (kind IN (...))`).
2. Напишите получатель, который возвращает `[]news.Item` (заголовок, ссылка, анонс, при наличии полный текст, дата, рубрики), по образцу `news.Fetch`.
3. Сохраняйте записи через `Ingestor.Store(ctx, source, item)`: так новый источник получает те же дедупликацию, полноту текста и обновления.
4. Добавьте вызов в `Ingestor.RunOnce` или отдельный цикл и тест разбора ответа источника.

## Аналитические материалы Альфа-Банка

1. Подготовьте текст материала (для PDF — `scripts/ocr_research.sh`) и положите его в `data/research/<имя>.txt`. Каждая страница начинается со строки `=== PAGE N`: по ней фрагменты привязываются к страницам, и ссылка на источник открывает нужную страницу.
2. Добавьте метаданные в `KnownDocs` (`backend/internal/importer/research.go`): название, подзаголовок, дата, подразделение, признак устаревшего материала и темы. Без метаданных материал тоже загрузится, но в отчёте импорта будет `unknown_document_metadata`, а название возьмётся из имени файла.
3. Запустите `importer -only research`. Материал разбивается на фрагменты (`research_chunks`) с привязкой к странице и становится доступен инструменту агента `search_research`.

## Отрасль

Отрасль компании определяется по ОКВЭД. Чтобы система учитывала новую отрасль:

1. **Темы отрасли** — `divisionTopics` (двузначный класс) и `codeTopics` (подкласс) в `backend/internal/companies/okved.go`. Темы попадают в профиль компании и в предварительное ранжирование.
2. **Словарь тем** — если отрасли нужна новая тема новостей, добавьте её (см. «Тема новостей» ниже).
3. **Чувствительности** — если признаки компании этой отрасли создают особый канал влияния, добавьте правило `Exposure` в `companies.Build` (`backend/internal/companies/profile.go`): код, название, объяснение, темы и ID фактов `C:*`, на которых оно основано. Чувствительности передаются модели и используются в правилах.
4. **Тесты** — добавьте компанию отрасли в `TestIndustryAndFactDifferences` (`backend/internal/agent/personalization_test.go`) и в holdout `data/evaluation/companies.json` с разметкой в `expected.json`.

## Тип клиента и признаки компании

- Новые значения `CL_TYPE` расшифровываются в `clientTypeLabels` (`companies/profile.go`).
- Новый столбец CSV: колонка в `companies` (миграция), разбор в `backend/internal/importer/parse.go` и `load.go`, факт `add("id", "Подпись", значение, "КОЛОНКА")` в `companies.Build`. Факт получает ID `C:id`, автоматически попадает в реестр источников агента и проверяется валидатором `used_company_factors`.
- Сведения, которые вводит пользователь или будущая интеграция с Альфа-Бизнесом, хранятся в `companies.profile`. Их схема описана в `BusinessProfile` (`companies/business.go`), сами сведения превращаются в факты и чувствительности в `addBusiness`.

## Роль пользователя

1. Добавьте роль в `roles.All` (`backend/internal/roles/roles.go`): код, название, фокус (что важно этой роли — попадает в промпты) и темы (для правил ранжирования).
2. Интерфейс получает роли из `GET /api/roles`, отдельной правки фронтенда не нужно.
3. Добавьте роль в `TestRoleDifferences` и, при необходимости, запрос с этой ролью в holdout `data/evaluation/expected.json`.

## Тема новостей

1. Миграция: `INSERT INTO topics (code, name, description) VALUES (...)`.
2. Словарь основ слов для разметки без LLM — `lexicon` в `backend/internal/analysis/rules.go`.
3. Привязка темы к отраслям (`okved.go`), ролям (`roles.go`) и чувствительностям (`profile.go`).
4. Каталог тем для live-evaluation — `topicCatalog` в `backend/internal/evaluation/analyzers.go`.
5. Перезапустите API. Модель увидит новую тему в списке кодов при классификации, валидатор перестанет её отбрасывать, интерфейс получит её из `GET /api/topics`.

## Промпт

Промпты и JSON-схемы лежат в `backend/internal/analysis`:

| Шаг | Функция | Файл |
|---|---|---|
| Классификация | `ClassifyMessages` | `classify.go` |
| План | `PlanMessages` | `insight.go` |
| Анализ и доработка | `InsightMessages`, `insightSchema`, `completenessRule` | `insight.go` |
| Самопроверка | `CheckMessages` | `insight.go` |
| Оценка ленты | `BriefMessages` | `insight.go` |
| Вопросы к анализу | `QuestionMessages`, `answerSchema` | `questions.go` |

Порядок изменения:

1. Измените промпт. Новое поле ответа добавьте в Go-структуру, схему в промпте, валидатор, тип во фронтенде и OpenAPI.
2. Увеличьте `PromptVersion` (`classify.go`) или, для вопросов к анализу, `QAPromptVersion` (`questions.go`). Версия входит в `input_hash` анализа и в ключ кэша ответов: старые результаты пересчитаются при следующем обращении, а метрики в `/analytics/quality` разделятся по версиям.
3. Классификация кэшируется в `articles.classification` без привязки к версии. Если меняется формат классификации, сбросьте её: `UPDATE articles SET classification = NULL, classified_at = NULL;`.
4. Обновите заглушки модели в тестах (`internal/server/integration_test.go`, при необходимости `cmd/eval`), прогоните offline-evaluation и ограниченный live-прогон с `-baseline` (см. [OPERATIONS.md](OPERATIONS.md)).

## Валидатор

Валидаторы — детерминированный код, который проверяет и исправляет ответ модели без повторного вызова LLM. Они вызываются из `ValidateInsight` (`backend/internal/analysis/insight.go`) в таком порядке:

1. диапазоны и перечисления, известные темы;
2. ссылки на источники: факты — только `A*`/`N*`, позиция банка — только `R*`, неизвестные ID удаляются;
3. основание рисков и возможностей (`fact`, `bank_view`, `model_inference`);
4. действия — `validateActions` (`content.go`): общие советы и действия без объекта или причины удаляются;
5. признаки компании — `validateFactors` (`content.go`);
6. проверка фактов — `FactCheckInsight` (`factcheck.go`): проценты, суммы, сроки, даты, названия;
7. список использованных источников и `source_coverage`.

Чтобы добавить валидатор:

1. Напишите функцию `func validateX(in *Insight, reg *Registry) []string` в пакете `analysis`. Она исправляет `in` и возвращает человекочитаемые заметки.
2. Вызовите её в `ValidateInsight` в нужном месте и добавьте заметки в `notes`: они попадут в `validation_notes`, в трассу и в счётчик «Исправления валидатора».
3. Если нарушение должно запускать доработку агентом, верните его как `CheckIssue{Severity: "high"}` из функции по образцу `ContentIssues` или `FactCheckIssues` и добавьте её вызов рядом с ними в `agent.run`.
4. Покройте функцию unit-тестом в `backend/internal/analysis` (образцы — `content_test.go`, `factcheck_test.go`).
5. Если валидатор даёт новую метрику качества, добавьте агрегат в `UserQualityAnalysisStats` (`backend/queries/analytics.sql`), поле в ответ `GET /api/analytics/quality` и карточку в `frontend/src/features/analytics/quality.tsx`.

## Быстрый вопрос к анализу

1. Добавьте вопрос в `QuickQuestions` (`backend/internal/analysis/questions.go`).
2. Если ответ можно собрать из сохранённого анализа без модели, поставьте `Deterministic: true` и реализуйте ветку в `DeterministicAnswer`. Иначе вопрос уйдёт в модель с тем же контекстом и валидатором `ValidateAnswer`.
3. Добавьте подпись вопроса в `presetLabel` (`frontend/src/features/analytics/quality.tsx`) и тест в `questions_test.go`.

## Модель

Разрешены только модели из Приложения 3 кейса, список — `Allowed` в `backend/internal/llm/allowed.go`. Добавлять туда модели вне списка кейса нельзя. Выкатка и откат модели описаны в [OPERATIONS.md](OPERATIONS.md#выкатка-новой-модели).
