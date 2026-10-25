-- +goose Up
-- +goose StatementBegin

-- =========================================================================
-- Справочники
-- =========================================================================

-- Темы новостей и интересов. Фиксированный список, чтобы классификация
-- моделью и фильтры в интерфейсе использовали одни и те же коды.
CREATE TABLE topics (
    code        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

-- =========================================================================
-- Данные кейса (импорт из CSV организаторов, значения сохраняются как есть)
-- =========================================================================

-- df_Uid_data_hack.csv. Исходные колонки сохраняются 1:1; флаги 0.0/1.0
-- приводятся к boolean, пустые строки -> NULL.
CREATE TABLE companies (
    id                       TEXT PRIMARY KEY,            -- SYNTHETIC_U
    org_type                 TEXT,                        -- ORG_TYPE
    okfs_name                TEXT,                        -- OKFS_NAME
    company_age              TEXT,                        -- COMPANY_AGE_YEARS_GRADATION
    eco_stage                TEXT,                        -- ECO_MARK_LABEL
    okved_code               TEXT NOT NULL,               -- MAINOKVED_CODE
    okved_name               TEXT NOT NULL,               -- MAINOKVED_NAME
    okved_division           TEXT NOT NULL,               -- первые 2 цифры ОКВЭД (производное)
    negative_mark            BOOLEAN,                     -- NEGATIVEMARK
    workers_fns              TEXT,                        -- AMOUNT_WORKERS_FNS_GRADATION
    workers_spark            TEXT,                        -- AMOUNT_WORKERS_SPARK_GRADATION
    client_type              TEXT,                        -- CL_TYPE
    has_rko                  BOOLEAN,                     -- CL_PRODUCT_RKO_MARK
    has_guarantee            BOOLEAN,                     -- CL_PRODUCT_GAR_MARK
    has_loan                 BOOLEAN,                     -- CL_PRODUCT_LOA_MARK
    has_leasing              BOOLEAN,                     -- CL_PRODUCT_LEA_MARK
    has_deposit              BOOLEAN,                     -- CL_PRODUCT_DEP_MARK
    has_payroll              BOOLEAN,                     -- CL_PRODUCT_ZPP_MARK
    fts_flag                 BOOLEAN,                     -- FTS_FLAG
    fts_countries            TEXT[] NOT NULL DEFAULT '{}',-- FTS_COUNTRY_NAME (разбит по ", ")
    has_factoring            BOOLEAN,                     -- FACTORING_MARK
    revenue                  NUMERIC(20,2),               -- REVENUE_COMPANY, руб.
    group_id                 TEXT,                        -- GROUP_ID
    city                     TEXT,                        -- CITY
    region                   TEXT,                        -- REGION
    source_row               INTEGER NOT NULL,            -- номер строки в CSV
    -- Поля, которые пользователь может уточнить. Исходные значения выше
    -- не перезаписываются.
    display_name             TEXT,
    profile                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    imported_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX companies_okved_division_idx ON companies (okved_division);
CREATE INDEX companies_region_idx ON companies (region);
CREATE INDEX companies_group_idx ON companies (group_id);
CREATE INDEX companies_revenue_idx ON companies (revenue DESC);

-- df_Fid_Uid_link_hack.csv: связь обезличенного физлица с компанией.
-- Внешний ключ на companies намеренно не ставится: ~63% ссылок указывают
-- на компании, которых нет в df_Uid_data_hack.csv; они сохраняются как есть.
CREATE TABLE person_links (
    person_id   TEXT NOT NULL,   -- SYNTHETIC_F
    company_id  TEXT NOT NULL,   -- SYNTHETIC_U
    city_name   TEXT,            -- CITY_NAME
    region_name TEXT,            -- RC_NAME
    source_row  INTEGER NOT NULL,
    PRIMARY KEY (person_id, company_id)
);
CREATE INDEX person_links_company_idx ON person_links (company_id);

-- df_Uid_payment_hack.csv: агрегированные платежи между компаниями.
CREATE TABLE payments (
    id           BIGSERIAL PRIMARY KEY,
    payer        TEXT NOT NULL,           -- PAYER
    beneficiary  TEXT NOT NULL,           -- BENEFICIARY
    amount       NUMERIC(20,2) NOT NULL,  -- PAY_AMT
    pay_count    INTEGER NOT NULL,        -- PAY_CNT
    is_internal  BOOLEAN NOT NULL,        -- payer = beneficiary
    source_row   INTEGER NOT NULL
);
CREATE INDEX payments_payer_idx ON payments (payer);
CREATE INDEX payments_beneficiary_idx ON payments (beneficiary);

-- Журнал проблем импорта: дубликаты, несоответствия типов и т.п.
CREATE TABLE import_issues (
    id          BIGSERIAL PRIMARY KEY,
    file_name   TEXT NOT NULL,
    source_row  INTEGER,
    kind        TEXT NOT NULL,
    detail      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================================================================
-- Пользователи
-- =========================================================================

CREATE TABLE users (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email                 TEXT NOT NULL,
    password_hash         TEXT NOT NULL,
    full_name             TEXT NOT NULL DEFAULT '',
    role_code             TEXT NOT NULL DEFAULT '',      -- линза персонализации (cfo, procurement, ...)
    position_title        TEXT NOT NULL DEFAULT '',      -- должность как ввёл пользователь
    work_focus            TEXT NOT NULL DEFAULT '',      -- свободное описание рабочих интересов
    personalization       JSONB NOT NULL DEFAULT '{}'::jsonb,
    onboarding_completed  BOOLEAN NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(email));

-- Refresh-сессии. Хранится только хеш токена.
CREATE TABLE sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    user_agent    TEXT NOT NULL DEFAULT '',
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

-- Членство пользователя в компании. Пользователь может состоять в нескольких
-- компаниях, в компании может быть несколько пользователей.
CREATE TABLE memberships (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_id  TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    is_primary  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, company_id)
);
CREATE UNIQUE INDEX memberships_one_primary_idx ON memberships (user_id) WHERE is_primary;

CREATE TABLE user_interests (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    topic_code  TEXT NOT NULL REFERENCES topics(code),
    weight      SMALLINT NOT NULL DEFAULT 1 CHECK (weight BETWEEN 1 AND 3),
    PRIMARY KEY (user_id, topic_code)
);

-- =========================================================================
-- Источники и материалы
-- =========================================================================

CREATE TABLE sources (
    id          SERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('rss','alfa_research','user_submitted')),
    feed_url    TEXT,
    homepage    TEXT,
    terms_note  TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    last_fetch_at     TIMESTAMPTZ,
    last_fetch_status TEXT
);

-- Аналитические материалы Альфа-Банка (PDF кейса, текст получен OCR).
CREATE TABLE research_documents (
    id            SERIAL PRIMARY KEY,
    file_name     TEXT NOT NULL UNIQUE,
    title         TEXT NOT NULL,
    subtitle      TEXT NOT NULL DEFAULT '',
    published_on  DATE,
    author_unit   TEXT NOT NULL DEFAULT '',
    pages         INTEGER NOT NULL DEFAULT 0,
    is_outdated   BOOLEAN NOT NULL DEFAULT false,
    lead_text     TEXT NOT NULL DEFAULT '',
    imported_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE research_chunks (
    id           SERIAL PRIMARY KEY,
    document_id  INTEGER NOT NULL REFERENCES research_documents(id) ON DELETE CASCADE,
    chunk_no     INTEGER NOT NULL,
    page         INTEGER NOT NULL,
    content      TEXT NOT NULL,
    tsv          TSVECTOR GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED,
    UNIQUE (document_id, chunk_no)
);
CREATE INDEX research_chunks_tsv_idx ON research_chunks USING GIN (tsv);

-- Новостные статьи. Хранится то, что отдаёт источник (заголовок, анонс,
-- ссылка, дата); полные тексты с сайтов не копируются.
CREATE TABLE articles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id      INTEGER NOT NULL REFERENCES sources(id),
    url            TEXT NOT NULL,
    guid           TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL,
    summary        TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL DEFAULT '',
    published_at   TIMESTAMPTZ NOT NULL,
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    title_norm     TEXT NOT NULL,
    content_hash   TEXT NOT NULL,
    duplicate_of   UUID REFERENCES articles(id),
    submitted_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    classified_at  TIMESTAMPTZ,
    classification JSONB,
    tsv            TSVECTOR GENERATED ALWAYS AS (
                     setweight(to_tsvector('russian', coalesce(title,'')), 'A') ||
                     setweight(to_tsvector('russian', coalesce(summary,'')), 'B') ||
                     setweight(to_tsvector('russian', coalesce(body,'')), 'C')) STORED
);
CREATE UNIQUE INDEX articles_url_idx ON articles (url);
CREATE INDEX articles_published_idx ON articles (published_at DESC);
CREATE INDEX articles_hash_idx ON articles (content_hash);
CREATE INDEX articles_title_norm_idx ON articles (title_norm);
CREATE INDEX articles_tsv_idx ON articles USING GIN (tsv);

CREATE TABLE article_topics (
    article_id  UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    topic_code  TEXT NOT NULL REFERENCES topics(code),
    confidence  REAL NOT NULL DEFAULT 1,
    PRIMARY KEY (article_id, topic_code)
);
CREATE INDEX article_topics_topic_idx ON article_topics (topic_code);

-- =========================================================================
-- ИИ-анализ
-- =========================================================================

-- Каждый запуск модели/агента с трассой шагов.
CREATE TABLE analysis_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind          TEXT NOT NULL CHECK (kind IN ('classify','rank','analyze','ingest')),
    status        TEXT NOT NULL CHECK (status IN ('running','succeeded','failed')),
    user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    company_id    TEXT REFERENCES companies(id) ON DELETE SET NULL,
    article_id    UUID REFERENCES articles(id) ON DELETE SET NULL,
    models        TEXT[] NOT NULL DEFAULT '{}',
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    steps         JSONB NOT NULL DEFAULT '[]'::jsonb,
    error         TEXT,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);
CREATE INDEX analysis_runs_started_idx ON analysis_runs (started_at DESC);
CREATE INDEX analysis_runs_article_idx ON analysis_runs (article_id);

-- Персональный анализ новости для пользователя (в контексте компании).
-- depth = 'brief'  — оценка релевантности из ранжирования ленты;
-- depth = 'full'   — полный анализ агентом с источниками.
CREATE TABLE personal_analyses (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id          UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    company_id          TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    role_code           TEXT NOT NULL,
    depth               TEXT NOT NULL CHECK (depth IN ('brief','full')),
    method              TEXT NOT NULL CHECK (method IN ('llm','rules')),
    input_hash          TEXT NOT NULL,
    relevance_score     SMALLINT NOT NULL CHECK (relevance_score BETWEEN 0 AND 100),
    importance_level    TEXT NOT NULL CHECK (importance_level IN ('low','medium','high','critical')),
    impact_type         TEXT NOT NULL DEFAULT 'neutral' CHECK (impact_type IN ('risk','opportunity','mixed','neutral')),
    matched_topics      TEXT[] NOT NULL DEFAULT '{}',
    why_it_matters      TEXT NOT NULL DEFAULT '',
    result              JSONB NOT NULL DEFAULT '{}'::jsonb,
    run_id              UUID REFERENCES analysis_runs(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (article_id, user_id, depth)
);
CREATE INDEX personal_analyses_user_idx ON personal_analyses (user_id, relevance_score DESC);

CREATE TABLE saved_articles (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id  UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, article_id)
);

CREATE TABLE feedback (
    id           BIGSERIAL PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id   UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    analysis_id  UUID REFERENCES personal_analyses(id) ON DELETE SET NULL,
    rating       SMALLINT NOT NULL CHECK (rating IN (-1, 1)),
    comment      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, article_id)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feedback, saved_articles, personal_analyses, analysis_runs,
    article_topics, articles, research_chunks, research_documents, sources,
    user_interests, memberships, sessions, users, import_issues, payments,
    person_links, companies, topics CASCADE;
-- +goose StatementEnd
