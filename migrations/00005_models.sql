-- Model management: a provider also serves embedding models and rerankers,
-- set up in the settings screen beside its chat models.

-- +goose Up
ALTER TABLE llm_providers ADD COLUMN embeddings jsonb NOT NULL DEFAULT '[]';
ALTER TABLE llm_providers ADD COLUMN rerankers jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE llm_providers DROP COLUMN rerankers;
ALTER TABLE llm_providers DROP COLUMN embeddings;
