-- Phase 4: accounts and sessions, feedback review, answer evaluation runs.

-- +goose Up
ALTER TABLE users ADD COLUMN disabled boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN last_login timestamptz;

-- A session is a random token held in an HttpOnly cookie; only its SHA-256
-- is stored, so a database dump does not hand out live sessions.
CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen  timestamptz NOT NULL DEFAULT now(),
    user_agent text NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions (user_id);

-- Review of feedback: who handled it, what was found, and the gold item it
-- became.
ALTER TABLE feedback ADD COLUMN status text NOT NULL DEFAULT 'open'; -- open, resolved, dismissed
ALTER TABLE feedback ADD COLUMN reviewer_id bigint REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE feedback ADD COLUMN review_note text NOT NULL DEFAULT '';
ALTER TABLE feedback ADD COLUMN gold_item_id bigint REFERENCES gold_items(id) ON DELETE SET NULL;
ALTER TABLE feedback ADD COLUMN reviewed_at timestamptz;
CREATE INDEX feedback_status ON feedback (status, created_at DESC);

-- End-to-end answer evaluation (/lab/answers).
CREATE TABLE answer_runs (
    id             bigserial PRIMARY KEY,
    label          text NOT NULL DEFAULT '',
    gold_set_id    bigint REFERENCES gold_sets(id) ON DELETE SET NULL,
    corpus_version bigint REFERENCES corpus_versions(id),
    config         jsonb NOT NULL,           -- the answer and router targets used
    status         text NOT NULL,            -- running, done, failed
    progress       integer NOT NULL DEFAULT 0,
    total          integer NOT NULL DEFAULT 0,
    metrics        jsonb,
    error          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz
);
CREATE TABLE answer_results (
    run_id       bigint NOT NULL REFERENCES answer_runs(id) ON DELETE CASCADE,
    gold_item_id bigint NOT NULL REFERENCES gold_items(id) ON DELETE CASCADE,
    result       jsonb NOT NULL,             -- the answer: claims, refusal, cards, steps
    metrics      jsonb NOT NULL,
    PRIMARY KEY (run_id, gold_item_id)
);

ALTER TABLE ingest_jobs ADD COLUMN progress text NOT NULL DEFAULT '';
ALTER TABLE ingest_jobs ADD COLUMN started_at timestamptz;
ALTER TABLE ingest_jobs ADD COLUMN user_id bigint REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE usage ADD COLUMN conversation_id bigint REFERENCES conversations(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE usage DROP COLUMN conversation_id;
ALTER TABLE ingest_jobs DROP COLUMN user_id;
ALTER TABLE ingest_jobs DROP COLUMN started_at;
ALTER TABLE ingest_jobs DROP COLUMN progress;
DROP TABLE answer_results, answer_runs;
DROP INDEX feedback_status;
ALTER TABLE feedback DROP COLUMN reviewed_at;
ALTER TABLE feedback DROP COLUMN gold_item_id;
ALTER TABLE feedback DROP COLUMN review_note;
ALTER TABLE feedback DROP COLUMN reviewer_id;
ALTER TABLE feedback DROP COLUMN status;
DROP TABLE sessions;
ALTER TABLE users DROP COLUMN last_login;
ALTER TABLE users DROP COLUMN disabled;
