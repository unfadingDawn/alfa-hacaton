# Сопровождение: выкатка, проверка изменений, откат

Документ для того, кто сопровождает AlfaInsight после хакатона. Команды даны для Docker Compose из корня репозитория. Без Docker используйте те же утилиты из `backend` через `go run ./cmd/<утилита>`.

## Версии и что в них входит

| Что | Где задаётся | Как отследить |
|---|---|---|
| Версия кода | git-тег `vX.Y.Z` на release-коммите | `git describe --tags` |
| Схема БД | goose-миграции `backend/migrations`, API применяет их при старте | `docker compose run --rm api migrate status` |
| Модели | `LLM_MODEL_FAST`, `LLM_MODEL_REASONING`, `LLM_MODEL_WRITER` в `.env` | лог `starting` при запуске API, таблица «Модели» в `/analytics/quality` |
| Промпт анализа | `PromptVersion` в `backend/internal/analysis/classify.go` | поле `prompt_version` в анализе, таблица «Версии промпта» в `/analytics/quality` |
| Промпт вопросов к анализу | `QAPromptVersion` в `backend/internal/analysis/questions.go` | `analysis_questions.prompt_version` |
| Holdout-набор | `data/evaluation/*.json` (поле `version`) | `dataset_version` в отчёте evaluation |

Анализ кэшируется по `input_hash`, в который входят версия промпта, writer-модель, версия статьи и профиль пользователя. Поэтому смена модели или промпта не требует ручной очистки: старые анализы пересчитываются при следующем открытии, а не все сразу, и бюджет не сгорает.

## Проверка изменений перед релизом

Перед слиянием изменения (pull request или набор коммитов) пройдите по списку.

**Автоматические проверки** (обязательны):

```bash
cd backend
gofmt -l .                 # пусто
go vet ./...
go test ./...              # unit-тесты, mock LLM, без расхода ключа
TEST_DATABASE_URL='postgres://.../alfainsight_test?sslmode=disable' \
  go test ./internal/server -run '^TestEndToEnd$'   # отдельная тестовая БД, схема пересоздаётся
go run ./cmd/eval -data ../data/evaluation -mode mock -out ../evaluation-results/candidate-mock.json \
  -baseline ../evaluation-results/release-mock.json

cd ../frontend
pnpm typecheck
pnpm build
```

Интеграционный тест проходит сквозной сценарий через HTTP API с настоящей PostgreSQL и заглушкой модели. Попутно он проверяет, что все миграции откатываются до нуля и применяются заново.

**Ручная проверка по типу изменения:**

| Если изменились | Проверьте |
|---|---|
| миграции | есть раздел `Down`; уже применённые файлы не менялись; `migrate down-to <предыдущая>` и `migrate up` на копии БД |
| `queries/*.sql` | выполнен `sqlc generate`, сгенерированный код закоммичен |
| промпты или схема ответа | увеличен `PromptVersion` или `QAPromptVersion`; обновлены заглушки в тестах; выполнен ограниченный live-evaluation с `-baseline` |
| валидаторы | есть unit-тест; новые заметки видны в трассе; метрика добавлена в `/analytics/quality`, если она нужна |
| API | обновлены `backend/api/openapi.yaml` и `frontend/src/lib/types.ts` |
| модели в `.env.example` | модель есть в `llm/allowed.go` (список кейса); `llmcheck` её видит |
| зависимости | обновлены `go.sum` или `pnpm-lock.yaml`; новые компоненты внесены в `THIRD_PARTY_NOTICES.md` |
| источники новостей | условия использования внесены в `terms_note`; политика полного текста обоснована |
| секреты и данные | `.env` и `data/raw/` не попали в git: `git status --ignored`, `git ls-files | grep -E '^\.env$|data/raw'` |

**Live-evaluation** запускайте только при изменении промпта, модели или валидатора: он расходует общий бюджет ключа.

```bash
set -a; . ./.env; set +a
cd backend
go run ./cmd/eval -data ../data/evaluation -mode live -confirm-live -max-live-cases 5 \
  -out ../evaluation-results/candidate-live.json -baseline ../evaluation-results/release-live.json
```

Сравните `json_success_rate`, `score_in_range_rate`, `personalization_rate`, `hallucination_rate`, `source_coverage`, `unverified_value_rate` и `differentiation_rate`. Ухудшение любой метрики больше чем на 0,05 без понятной причины блокирует релиз.

## Выпуск версии

1. Пройдите проверку изменений выше.
2. Сохраните отчёты evaluation как новый baseline: `release-mock.json`, `release-live.json`.
3. Создайте release-коммит и аннотированный тег:

   ```bash
   git tag -a v1.1.0 -m "AlfaInsight 1.1.0: <кратко что изменилось>"
   git push origin v1.1.0
   ```

4. Сделайте резервную копию БД, разверните и выполните smoke-проверку по [DEPLOYMENT.md](DEPLOYMENT.md).
5. В течение первых суток смотрите `/analytics/quality`: success rate, ошибки JSON, обрезанные ответы, покрытие источниками, доля неподтверждённых значений, helpfulness.

## Выкатка новой модели

Используются только модели из списка кейса (`backend/internal/llm/allowed.go`).

1. **Доступность без расхода токенов:** `docker compose run --rm api llmcheck` — список моделей шлюза и отметка, какие разрешены.
2. **Один тестовый запрос:** `docker compose run --rm -e LLM_MODEL_FAST=<новая модель> api llmcheck -test`. Утилита отправляет одну классификацию и печатает результат после разбора и валидации. Если вместо результата пришёл обрезанный ответ, модель тратит `max_tokens` на скрытое рассуждение и для структурированных шагов не подходит.
3. **Сравнение качества:** live-evaluation с `-model <новая модель> -max-live-cases 5 -baseline <отчёт текущей модели>`.
4. **Переключение:** измените нужную переменную в `.env` (`LLM_MODEL_FAST` — классификация и оценка ленты, `LLM_MODEL_REASONING` — план и самопроверка, `LLM_MODEL_WRITER` — анализ и вопросы) и пересоздайте контейнер API: `docker compose up -d --force-recreate api`. Обычный `restart` не перечитывает `.env`.
5. **Наблюдение:** в таблице «Модели» в `/analytics/quality` у новой модели не должно расти число ошибок; success rate и доля ошибок JSON не должны ухудшаться.
6. **Откат модели:** верните прежнее значение в `.env` и снова выполните `docker compose up -d --force-recreate api`. Схема БД и данные от модели не зависят, анализы с новой моделью пересчитаются при открытии.

Сменить шлюз модели (резервный URL организаторов) можно так же, через `LLM_BASE_URL`.

## Откат версии

Порядок важен: схему откатывают **новой** версией утилиты `migrate`, потому что только она знает SQL отката миграций, добавленных в этом релизе.

1. **Резервная копия:**

   ```bash
   docker compose exec db pg_dump -U alfa -Fc alfainsight > backup-$(date +%Y%m%d-%H%M).dump
   ```

2. **Какие миграции добавил релиз:**

   ```bash
   docker compose run --rm api migrate status
   git diff --name-only v1.0.0 v1.1.0 -- backend/migrations
   ```

3. **Откат схемы до версии предыдущего релиза** (номер — последняя миграция предыдущего тега):

   ```bash
   docker compose stop api web
   docker compose run --rm api migrate down-to 9
   ```

   Откат удаляет таблицы и колонки, добавленные релизом, вместе с их данными (например, `analysis_questions`). Поэтому копия из шага 1 обязательна.

4. **Развёртывание предыдущей версии:**

   ```bash
   git checkout v1.0.0
   docker compose up -d --build
   docker compose ps
   ```

5. **Smoke-проверка** по [DEPLOYMENT.md](DEPLOYMENT.md#smoke-проверка-после-развёртывания).

Если релиз не менял схему, достаточно шага 4. Чтобы восстановить БД из копии целиком: `docker compose exec -T db pg_restore -U alfa -d alfainsight --clean --if-exists < backup.dump`.

## Регулярный мониторинг качества

Раз в неделю или после каждого релиза:

1. Откройте `/analytics/quality` за 7 дней. Проверьте стабильность (ошибки JSON, обрезанные ответы, таймауты, лимиты), фактологическую точность (покрытие источниками, неподтверждённые значения), содержательность (механизм через признаки компании, удалённые общие советы), уточняющие вопросы (доля ответов с источниками, «нет в источниках») и helpfulness.
2. Разберите отрицательные отзывы и неуспешные запуски через трассу анализа («Как получен вывод»). Определите причину: ранжирование, grounding, персонализация, конкретность действий или UX.
3. Исправьте правило, валидатор или промпт (см. [EXTENDING.md](EXTENDING.md)), увеличьте версию промпта и пройдите проверку изменений.
4. После выпуска сравните метрики новой и предыдущей версии промпта в таблице «Версии промпта».

Комментарии из отзывов не отправляются в модель автоматически. Решение об изменении принимает человек.
