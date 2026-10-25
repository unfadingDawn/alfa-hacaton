# Архитектура AlfaInsight

Документ описывает компоненты, агентный конвейер, модель данных, механизмы безопасности и подход к масштабированию. Как расширять систему — в [EXTENDING.md](EXTENDING.md), как выкатывать и откатывать — в [OPERATIONS.md](OPERATIONS.md), чек-лист развёртывания — в [DEPLOYMENT.md](DEPLOYMENT.md).

## Компоненты

```mermaid
flowchart LR
  subgraph Client["Браузер"]
    UI["Next.js 16<br/>лента, карточка новости,<br/>вопросы к анализу, качество AI"]
  end

  subgraph Web["web (Node.js)"]
    Proxy["same-origin /api/*<br/>прокси на backend"]
  end

  subgraph API["api (Go, chi)"]
    Auth["auth: JWT в HttpOnly cookie,<br/>Origin guard, rate limit входа"]
    Insights["insights: лента, карточка,<br/>анализ, вопросы к анализу"]
    Analytics["analytics: главная,<br/>бизнес-срезы, качество AI"]
    Agent["agent: агентный конвейер"]
    Rules["analysis: промпты, валидаторы,<br/>проверка фактов, правила ранжирования"]
    Semantic["semantic: TF-IDF + LSA<br/>(в памяти, без LLM)"]
    Companies["companies: профиль компании,<br/>факты C:*, чувствительности"]
    LLMClient["llm: allowlist моделей, кэш,<br/>бюджет, лимиты, пауза после 429"]
    Ingest["news: RSS, полнота текста,<br/>дедупликация, обновления"]
  end

  DB[("PostgreSQL 16<br/>companies, articles, research_chunks,<br/>personal_analyses, analysis_runs,<br/>analysis_questions, feedback")]
  LLM["OpenAI-совместимый шлюз<br/>разрешённые модели кейса"]
  RSS["RSS: Банк России, РБК, Интерфакс,<br/>ТАСС, Ведомости, Финмаркет, MOEX"]
  CSV["CSV кейса и тексты<br/>аналитики Альфа-Банка"]
  Importer["importer (разовый запуск)"]

  UI --> Proxy --> Auth
  Auth --> Insights & Analytics
  Insights --> Agent
  Agent --> Rules & Companies & LLMClient
  Agent -->|"инструменты: чтение"| DB
  Insights --> Semantic
  LLMClient -->|"HTTPS, ключ только в заголовке"| LLM
  Ingest --> RSS
  Ingest --> DB
  Importer --> CSV
  Importer --> DB
  Analytics --> DB
  Insights --> DB
```

| Компонент | Код | Назначение |
|---|---|---|
| Frontend | `frontend/src/features/*` | Пользовательский сценарий: онбординг, лента, карточка новости, анализ, трасса, вопросы, аналитика, качество AI |
| HTTP API | `backend/internal/server`, `insights`, `analytics`, `users`, `companies`, `auth` | REST API, контракт — `backend/api/openapi.yaml` |
| Агент | `backend/internal/agent` | Оркестрация шагов, инструменты поиска, кэш анализов, трасса |
| Аналитическое ядро | `backend/internal/analysis` | Промпты и JSON-схемы, валидаторы, проверка фактов, содержательность, правила ранжирования, отчёт трассы |
| Профиль компании | `backend/internal/companies` | Факты `C:*` из CSV и данных пользователя, чувствительности бизнеса |
| Клиент LLM | `backend/internal/llm` | Allowlist, кэш, объединение одинаковых запросов, дневной бюджет, лимиты частоты |
| Сбор новостей | `backend/internal/news` | RSS, нормализация, полнота текста, дедупликация, обновления публикаций |
| Семантика | `backend/internal/semantic` | Локальная модель TF-IDF + LSA для предварительного ранжирования |
| Утилиты | `backend/cmd/*` | `api`, `importer`, `llmcheck`, `eval`, `migrate` |

## Агентный конвейер

Модель вызывается только по действию пользователя. Фоновые задачи (RSS, темы по словарю, TF-IDF + LSA) работают без LLM.

```mermaid
flowchart TD
  Start(["POST /articles/{id}/analyze"]) --> Cache{"input_hash совпадает?<br/>статья + её версия, профиль,<br/>роль, промпт, модель"}
  Cache -->|да| Done(["Сохранённый анализ"])
  Cache -->|нет| Classify["1. Классификация<br/>быстрая модель, кэш на статью"]
  Classify --> Registry["Реестр источников<br/>A1 новость + полнота текста,<br/>C:* факты компании"]
  Registry --> Plan["2. План<br/>вопросы, гипотезы, запросы"]
  Plan --> Tools["3. Инструменты (чтение БД)<br/>R* аналитика банка, N* связанные новости"]
  Tools --> Draft["4. Черновик<br/>строгий JSON"]
  Draft --> Validate["5. Валидация<br/>источники, позиция банка, действия,<br/>признаки компании"]
  Validate --> Facts["6. Проверка фактов<br/>проценты, суммы, сроки, даты, названия"]
  Facts --> Check["7. Самопроверка (LLM)<br/>+ замечания проверки фактов<br/>и содержательности"]
  Check -->|"нет серьёзных замечаний"| Save
  Check -->|"есть серьёзные"| More["Дополнительный поиск"]
  More --> Revise["8. Доработка (один раз)"]
  Revise --> Validate2["Валидация и проверка фактов"]
  Validate2 --> Save["9. Сохранение<br/>анализ, план, отчёт об источниках,<br/>итог самопроверки, трасса"]
  Save --> Done
```

Каждый шаг пишется в `analysis_runs.steps`: модель, токены, длительность, статус и структурированный результат. В трассу не попадают скрытые рассуждения модели: клиент отбрасывает блоки `<think>`, сохраняются только разобранные JSON-структуры.

Уточняющие вопросы (`POST /articles/{id}/analysis/questions`) работают поверх сохранённого анализа. Контекст ответа — анализ, план и его реестр источников. Два быстрых вопроса отвечаются без модели.

## Модель данных

```mermaid
erDiagram
  users ||--o{ memberships : ""
  companies ||--o{ memberships : ""
  users ||--o{ user_interests : ""
  topics ||--o{ user_interests : ""
  sources ||--o{ articles : ""
  articles ||--o{ article_topics : ""
  articles ||--o| articles : "duplicate_of"
  research_documents ||--o{ research_chunks : ""
  articles ||--o{ personal_analyses : ""
  users ||--o{ personal_analyses : ""
  analysis_runs ||--o{ personal_analyses : "run_id"
  personal_analyses ||--o{ analysis_questions : ""
  users ||--o{ feedback : ""
  personal_analyses ||--o{ feedback : ""

  companies { text id PK "SYNTHETIC_U" text okved_code jsonb profile }
  sources { text code text kind text full_text_policy text_array category_filter }
  articles { uuid id PK text title text summary text body text content_level "вычисляется БД" uuid duplicate_of timestamptz updated_at }
  personal_analyses { uuid id PK text depth "brief или full" text input_hash jsonb result }
  analysis_runs { uuid id PK text kind jsonb steps int prompt_tokens int completion_tokens }
  analysis_questions { uuid id PK text analysis_hash text question_norm jsonb answer text_array source_ids int tokens }
```

`personal_analyses.result` хранит `insight` (выводы с `source_ids`, `used_company_factors`, `fact_check`, `source_coverage`, `source_completeness`), `plan`, `classification`, `sources_report`, `revision`, версию промпта и модели.

## Механизмы безопасности AI-компонентов

| Требование кейса (ограничение 06) | Реализация | Где в коде |
|---|---|---|
| Разграничение прав доступа | Все запросы, кроме входа и публичной витрины, требуют JWT в HttpOnly cookie. Анализы, трассы, вопросы и добавленные вручную новости доступны только владельцу: выборка всегда по паре «объект + текущий пользователь». Компания видна только её участникам | `auth/middleware.go`, `insights/handlers.go`, `agent/questions.go`, `companies/handlers.go` |
| Защита чувствительных данных | Секреты только в `.env`; ключ LLM передаётся только backend в заголовке `Authorization` и маскируется в логах; пароли — Argon2id; в логах HTTP только метод, путь и статус; данные кейса обезличены; публичная витрина не раскрывает пользователей и компании | `platform/config`, `llm/client.go`, `auth/password.go`, `server/server.go` |
| Контроль и журналирование обращений к инструментам | Каждый шаг агента (вызов модели или инструмента) с запросами, найденными источниками, моделью, токенами и статусом пишется в `analysis_runs`; вопросы к анализу — в `analysis_questions`; агрегаты — в `/analytics/quality` | `agent/agent.go`, `agent/questions.go`, `queries/analytics.sql` |
| Ограничение потенциально небезопасных действий | Инструменты агента выполняют только параметризованные запросы чтения к данным приложения; модель не формирует SQL и не вызывает внешние сервисы; ответы модели проходят валидацию (неизвестные источники, неподтверждённые числа и позиция банка удаляются); текст вопроса пользователя изолирован в промпте от инструкций; разрешены только модели из списка кейса | `agent.TSQuery`, `analysis/*`, `llm/allowed.go` |
| Контроль расхода | Дневной бюджет токенов и вызовов, лимит в минуту, почасовой лимит анализов, лимиты вопросов, пауза после 429 или ошибки бюджета, кэш и объединение одинаковых запросов | `llm/guard.go`, `llm/client.go`, `agent` |

## Масштабирование

**Новые отрасли, типы клиентов и источники** добавляются данными и небольшими справочниками, без изменения конвейера: отраслевые темы ОКВЭД, словарь тем, роли, источники с политикой полного текста, материалы банка. Пошагово — в [EXTENDING.md](EXTENDING.md).

**Нагрузка.**

- API почти без состояния: сессии и данные в PostgreSQL. В памяти процесса живут кэш LLM (6 часов), защита от двойного запуска анализа, счётчики минутного лимита и почасового лимита анализов, а также модель TF-IDF + LSA. При нескольких репликах эти счётчики нужно перенести в общий Redis или PostgreSQL. Дневной бюджет токенов уже пересчитывается из БД (`TokensUsedSince`) и переживает перезапуск.
- Сбор RSS запускается в процессе API. При нескольких репликах его нужно вынести в отдельный worker (`importer -news` по расписанию) и отключить `NEWS_FETCH_ENABLED` в API.
- Поиск по аналитике и новостям — полнотекстовый `tsvector` с GIN-индексами. Для больших корпусов в образе уже есть pgvector, на него можно перейти без смены СУБД.
- Расход LLM ограничен на уровне каждого шага (`max_tokens`). Полные анализы и ответы кэшируются по `input_hash`, в том числе между пользователями одной компании и роли.

**Качество при масштабировании** контролируется через holdout-evaluation (`backend/cmd/eval`) и мониторинг `/analytics/quality` с разбивкой по моделям и версиям промпта. Цикл описан в [OPERATIONS.md](OPERATIONS.md).
