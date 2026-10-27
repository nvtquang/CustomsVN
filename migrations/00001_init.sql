-- The whole CustomsVN schema. Tables for later phases are created now so the
-- schema has one origin; each says which phase first writes it.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- One row per ingest that changed something. A retrieval run records the
-- version it measured, so two runs on different data are never compared as if
-- they were on the same.
CREATE TABLE corpus_versions (
    id              bigserial PRIMARY KEY,
    created_at      timestamptz NOT NULL DEFAULT now(),
    as_of           date NOT NULL,
    source_revision text NOT NULL,
    chunks_sha256   text NOT NULL,
    documents       integer NOT NULL,
    chunks          integer NOT NULL,
    added           integer NOT NULL,
    changed         integer NOT NULL,
    removed         integer NOT NULL
);

-- Every document the corpus holds a chunk of, original or consolidated.
CREATE TABLE documents (
    id              text PRIMARY KEY,
    official_number text NOT NULL,
    number_norm     text NOT NULL,
    doc_type        text NOT NULL DEFAULT '',
    title           text NOT NULL,
    issuing_body    text NOT NULL DEFAULT '',
    signed_on       date,
    effective_from  date,
    expired_on      date,
    force_status    text NOT NULL DEFAULT '',
    status          text NOT NULL,
    tier            smallint NOT NULL,
    source_kind     text NOT NULL,
    source_url      text NOT NULL DEFAULT '',
    subjects        text[] NOT NULL DEFAULT '{}',
    -- A consolidated text names its base; an original names the consolidated
    -- text that supersedes its wording.
    consolidates    text,
    consolidated_in text,
    amended_by      text[] NOT NULL DEFAULT '{}',
    text_source     text,
    content_hash    text NOT NULL,
    corpus_version  bigint NOT NULL REFERENCES corpus_versions(id)
);
CREATE INDEX documents_number_norm ON documents (number_norm);
CREATE INDEX documents_tier ON documents (tier);
CREATE INDEX documents_consolidated_in ON documents (consolidated_in);
CREATE INDEX documents_title_trgm ON documents USING gin (title gin_trgm_ops);

-- The structure tree of each document, down to points.
CREATE TABLE provisions (
    id          text PRIMARY KEY,
    document_id text NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    parent_id   text,
    kind        text NOT NULL,
    number      text NOT NULL DEFAULT '',
    heading     text NOT NULL DEFAULT '',
    text        text NOT NULL DEFAULT '',
    text_hash   text NOT NULL DEFAULT '',
    position    integer NOT NULL
);
CREATE INDEX provisions_document ON provisions (document_id, position);
CREATE INDEX provisions_parent ON provisions (parent_id);

-- The retrievable units.
CREATE TABLE chunks (
    id              text PRIMARY KEY,
    document_id     text NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    path            text[] NOT NULL DEFAULT '{}',
    text            text NOT NULL,
    chunk_kind      text NOT NULL DEFAULT 'text',
    tier            smallint NOT NULL,
    source_kind     text NOT NULL,
    force           text NOT NULL,
    subjects        text[] NOT NULL DEFAULT '{}',
    amendment_notes text[] NOT NULL DEFAULT '{}',
    consolidated_in text,
    rescue_reason   text,
    needs_review    boolean NOT NULL DEFAULT false,
    text_source     text,
    content_hash    text NOT NULL,
    corpus_version  bigint NOT NULL REFERENCES corpus_versions(id)
);
CREATE INDEX chunks_document ON chunks (document_id);
CREATE INDEX chunks_filter ON chunks (tier, source_kind);
CREATE INDEX chunks_subjects ON chunks USING gin (subjects);

-- Several embedding models live side by side so they can be compared. The
-- column has no fixed dimension; embed builds one HNSW index per model on a
-- cast to that model's dimension.
CREATE TABLE embedding_models (
    name       text PRIMARY KEY,
    provider   text NOT NULL,
    model      text NOT NULL,
    dims       integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE chunk_embeddings (
    chunk_id     text NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    model        text NOT NULL REFERENCES embedding_models(name) ON DELETE CASCADE,
    content_hash text NOT NULL,
    embedding    vector NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (chunk_id, model)
);

-- Citation, amendment and consolidation links. Walked with recursive CTEs.
CREATE TABLE edges (
    id             bigserial PRIMARY KEY,
    from_doc       text NOT NULL,
    from_provision text,
    to_doc         text,
    to_number      text NOT NULL DEFAULT '',
    kind           text NOT NULL,
    method         text NOT NULL
);
CREATE INDEX edges_from ON edges (from_doc, kind);
CREATE INDEX edges_to ON edges (to_doc, kind);

-- The Vietnamese HS nomenclature (TT 31/2022/TT-BTC). Phase 3 tool.
CREATE TABLE hs_codes (
    id                 bigserial PRIMARY KEY,
    code               text NOT NULL DEFAULT '',
    level              smallint NOT NULL,
    description_vi     text NOT NULL,
    description_en     text NOT NULL DEFAULT '',
    unit               text NOT NULL DEFAULT '',
    chapter            text NOT NULL DEFAULT '',
    parent_code        text NOT NULL DEFAULT '',
    parent_description text NOT NULL DEFAULT '',
    source_doc         text NOT NULL,
    position           integer NOT NULL
);
CREATE INDEX hs_codes_code ON hs_codes (code);
CREATE INDEX hs_codes_vi_trgm ON hs_codes USING gin (description_vi gin_trgm_ops);

-- Duty rates by HS line, schedule and validity. Loaded once an official
-- source of the schedules is chosen; until then empty, and the bot refuses
-- rate questions.
CREATE TABLE tariffs (
    id               bigserial PRIMARY KEY,
    hs_code          text NOT NULL,
    schedule         text NOT NULL,           -- export, mfn, normal, or an FTA code such as cptpp, evfta
    rate_text        text NOT NULL,           -- verbatim, e.g. "5", "*", "0 (2026)"
    rate_percent     numeric(7,3),            -- when the rate is a plain percentage
    specific_rate    text,                    -- an amount per unit, verbatim
    conditions       text NOT NULL DEFAULT '',
    valid_from       date NOT NULL,
    valid_to         date,
    source_doc       text NOT NULL,
    source_provision text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (valid_to IS NULL OR valid_to > valid_from)
);
CREATE INDEX tariffs_lookup ON tariffs (hs_code, schedule, valid_from);

-- Retrieval Testing (Phase 1).
CREATE TABLE gold_sets (
    id         bigserial PRIMARY KEY,
    name       text NOT NULL,
    version    integer NOT NULL,
    format     text NOT NULL,
    note       text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);
CREATE TABLE gold_items (
    id                  bigserial PRIMARY KEY,
    gold_set_id         bigint NOT NULL REFERENCES gold_sets(id) ON DELETE CASCADE,
    ext_id              text NOT NULL,
    question            text NOT NULL,
    expected_provisions text NOT NULL DEFAULT '',
    expected_ids        text[] NOT NULL DEFAULT '{}',
    expected_answer     text NOT NULL DEFAULT '',
    as_of_date          date,
    category            text NOT NULL DEFAULT '',
    should_refuse       boolean NOT NULL DEFAULT false,
    notes               text NOT NULL DEFAULT '',
    UNIQUE (gold_set_id, ext_id)
);
CREATE TABLE retrieval_runs (
    id             bigserial PRIMARY KEY,
    label          text NOT NULL DEFAULT '',
    gold_set_id    bigint REFERENCES gold_sets(id) ON DELETE SET NULL,
    corpus_version bigint REFERENCES corpus_versions(id),
    config         jsonb NOT NULL,
    status         text NOT NULL,             -- running, done, failed
    progress       integer NOT NULL DEFAULT 0,
    total          integer NOT NULL DEFAULT 0,
    metrics        jsonb,
    error          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz
);
CREATE TABLE retrieval_results (
    run_id       bigint NOT NULL REFERENCES retrieval_runs(id) ON DELETE CASCADE,
    gold_item_id bigint NOT NULL REFERENCES gold_items(id) ON DELETE CASCADE,
    hits         jsonb NOT NULL,
    ranks        integer[] NOT NULL DEFAULT '{}',
    metrics      jsonb NOT NULL,
    latency_ms   real NOT NULL,
    PRIMARY KEY (run_id, gold_item_id)
);

-- Settings and LLM providers. API keys are AES-GCM ciphertext under a master
-- key from the environment and are never returned to the browser.
CREATE TABLE settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE llm_providers (
    id          bigserial PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    kind        text NOT NULL,                -- anthropic, openai-compatible
    base_url    text NOT NULL DEFAULT '',
    api_key_enc bytea,
    models      jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Accounts (Phase 4).
CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    email         text NOT NULL UNIQUE,
    name          text NOT NULL DEFAULT '',
    role          text NOT NULL DEFAULT 'user', -- user, expert, admin
    password_hash text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Conversations (Phase 2).
CREATE TABLE conversations (
    id         bigserial PRIMARY KEY,
    user_id    bigint REFERENCES users(id) ON DELETE SET NULL,
    title      text NOT NULL DEFAULT '',
    as_of_date date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversations_user ON conversations (user_id, updated_at DESC);
CREATE TABLE messages (
    id              bigserial PRIMARY KEY,
    conversation_id bigint NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            text NOT NULL,            -- user, assistant
    content         text NOT NULL,
    refused         boolean NOT NULL DEFAULT false,
    trace           jsonb,
    corpus_version  bigint REFERENCES corpus_versions(id),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_conversation ON messages (conversation_id, created_at);
CREATE TABLE citations (
    id         bigserial PRIMARY KEY,
    message_id bigint NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    ordinal    integer NOT NULL,
    chunk_id   text NOT NULL,
    quote      text NOT NULL,
    verified   boolean NOT NULL,
    force      text NOT NULL,
    note       text NOT NULL DEFAULT ''
);
CREATE INDEX citations_message ON citations (message_id);
CREATE TABLE feedback (
    id         bigserial PRIMARY KEY,
    message_id bigint NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id    bigint REFERENCES users(id) ON DELETE SET NULL,
    rating     smallint NOT NULL CHECK (rating IN (-1, 1)),
    reason     text NOT NULL DEFAULT '',
    label      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Background jobs and usage (Phase 4).
CREATE TABLE ingest_jobs (
    id          bigserial PRIMARY KEY,
    kind        text NOT NULL,                -- prep, ingest, embed
    params      jsonb NOT NULL DEFAULT '{}',
    status      text NOT NULL,
    log         text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE TABLE usage (
    id            bigserial PRIMARY KEY,
    at            timestamptz NOT NULL DEFAULT now(),
    user_id       bigint REFERENCES users(id) ON DELETE SET NULL,
    provider      text NOT NULL,
    model         text NOT NULL,
    purpose       text NOT NULL,              -- answer, route, embed, rerank
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    cost_usd      numeric(12,6) NOT NULL DEFAULT 0
);
CREATE INDEX usage_at ON usage (at);

-- +goose Down
DROP TABLE usage, ingest_jobs, feedback, citations, messages, conversations, users,
    llm_providers, settings, retrieval_results, retrieval_runs, gold_items, gold_sets,
    tariffs, hs_codes, edges, chunk_embeddings, embedding_models, chunks, provisions,
    documents, corpus_versions;
