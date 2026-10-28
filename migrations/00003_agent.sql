-- Phase 3: the agent's tools need the status of any document, in the corpus
-- or not, the official relation graph with its labels, and the HS lines
-- searchable by meaning. An answer keeps its cards and agent steps.

-- +goose Up

-- Every document of the store, as the status tool reads it. The corpus keeps
-- only documents in force; this keeps the ones that ended too, so "is this
-- circular still in force" has an answer either way.
CREATE TABLE registry (
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
    source_url      text NOT NULL DEFAULT ''
);
CREATE INDEX registry_number ON registry (number_norm);
CREATE INDEX registry_title_trgm ON registry USING gin (title gin_trgm_ops);

-- The relation an edge stands for, normalised from vbpl.vn's label:
-- amends, replaces, repeals, expires, suspends, corrects, details, consolidates,
-- cites. Empty for pattern links found in the text.
ALTER TABLE edges ADD COLUMN relation text NOT NULL DEFAULT '';
ALTER TABLE edges ADD COLUMN label text NOT NULL DEFAULT '';
CREATE INDEX edges_relation ON edges (to_doc, relation);

-- HS descriptions embedded per model, keyed by the line's content so a
-- re-ingest that renumbers rows keeps them.
ALTER TABLE hs_codes ADD COLUMN content_hash text NOT NULL DEFAULT '';
CREATE INDEX hs_codes_hash ON hs_codes (content_hash);
CREATE TABLE hs_embeddings (
    content_hash text NOT NULL,
    model        text NOT NULL REFERENCES embedding_models(name) ON DELETE CASCADE,
    embedding    vector NOT NULL,
    PRIMARY KEY (content_hash, model)
);

ALTER TABLE messages ADD COLUMN cards jsonb;

-- +goose Down
ALTER TABLE messages DROP COLUMN cards;
DROP TABLE hs_embeddings;
DROP INDEX hs_codes_hash;
ALTER TABLE hs_codes DROP COLUMN content_hash;
DROP INDEX edges_relation;
ALTER TABLE edges DROP COLUMN label;
ALTER TABLE edges DROP COLUMN relation;
DROP TABLE registry;
