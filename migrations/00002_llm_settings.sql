-- Phase 2: a provider's key may come from an environment variable instead of
-- the encrypted column, and an assistant message keeps its claims structured.

-- +goose Up
ALTER TABLE llm_providers ADD COLUMN api_key_env text NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN claims jsonb;
ALTER TABLE messages ADD COLUMN as_of_date date;
CREATE INDEX conversations_title_trgm ON conversations USING gin (title gin_trgm_ops);

-- +goose Down
DROP INDEX conversations_title_trgm;
ALTER TABLE messages DROP COLUMN as_of_date;
ALTER TABLE messages DROP COLUMN claims;
ALTER TABLE llm_providers DROP COLUMN api_key_env;
