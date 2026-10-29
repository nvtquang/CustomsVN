# CustomsVN

Open-source (MIT) AI chatbot and agent for Vietnamese customs law. Go backend built on luatdo, React Web UI embedded in the binary, PostgreSQL 17 + pgvector, cloud LLMs. Every answer cites an article/clause/point and its force status, or refuses.

**The plan is `docs/PLAN.md`. Read the section for the current phase before starting work in it.**

## Current phase

Phase 4 — admin UI and polish: built; Go tests and 13 Playwright tests pass against a fake model (`docs/admin.md`). Left for Phase 4: the experts' week on the beta, every 👎 handled in the review screen. Still waiting on real API keys and the experts' gold set: the embedding benchmark (Phase 1), answer citation rate and time to first claim (Phase 2), the release thresholds (Phase 3). Decisions of 2026-10-27: legacy_content on by default; no PRs to luatdo; no tariff source (rate questions are refused). Decisions of 2026-10-29: the vbpl.vn crawl is in v1.0, monthly by default (`docs/crawl.md`); system prompts are edited in Settings, not in code; answers never look up HS codes; no generated admin password. Settings reworked 2026-10-28 (`docs/admin.md`): one settings window, models by type (chat, embedding, rerank), and a knowledge page with chunking (general or parent-child), index method, top K and score threshold; the defaults keep retrieval as it was. Next: Phase 5 (open-source v1.0 release).

## Commands

```sh
go build ./...
go test ./...
go run ./cmd/customsvn version
go run ./cmd/customsvn prep -as-of 2026-10-27   # needs LUATDO_DATA; about a minute
docker compose -f deploy/docker-compose.yml up -d   # dev Postgres + pgvector on :55432
go run ./cmd/customsvn ingest                    # needs CUSTOMSVN_DATABASE_URL
go run ./cmd/customsvn eval -goldset goldsets/provisional.csv
go run ./cmd/customsvn serve
go run ./cmd/customsvn mcp                       # MCP over stdio
go run ./cmd/customsvn user passwd -email admin -password P  # reset a password
go run ./cmd/customsvn crawl -max-pages 1           # vbpl.vn into a new revision; needs the crawler (docs/crawl.md)
docker compose up -d --build && docker compose run --rm app bootstrap   # whole app
cd web && npm ci && npm run build                # then rebuild the binary to embed the UI
cd web && E2E_ADMIN_PASSWORD=... npm run e2e    # Playwright, against a test server (web/e2e/README.md)
```

On the maintainer's machine Windows Application Control blocks the Go compiler and every freshly built binary, so all `go` commands (vet, build, test, run) go through `golang:1.27` under Docker: mount the repo at /src and the data at /data; the database is at host.docker.internal:55432. `gofmt` at `D:\Tools\go\bin` still runs locally.

Data lives outside the repo. Set `LUATDO_DATA` (on the maintainer's machine `D:\DevCache\luatdo`). Never commit data, embeddings or API keys.

## Rules that do not bend

- Never use a document that is no longer in force as grounds for current law. Filter by force status and expiry date, not by the status label alone: the source lags.
- An answer is shown only after every quote in it is verified verbatim against the cited chunk. No grounds, no answer.
- Prefer a consolidated text (VBHN) over the original it consolidates.
- Deterministic first: structure, identifiers, force and amendment links come from code, never from a model.
- Any change to retrieval runs through Retrieval Testing, with metrics before and after in the PR.
- Config files hold no secrets. API keys are encrypted at rest and never sent to the browser.

## luatdo

- Imported as a Go module, pinned to a commit in `go.mod`. Do not vendor or copy its code.
- No PRs to luatdo (owner's decision, 2026-10-27). Work around a luatdo bug inside CustomsVN and note it where the workaround lives.
- Packages used: `parse`, `law`, `cite`, `subject`, `retrieve`, `route`, `api`, `eval`, `store`.

## Conventions

- Layout follows `docs/PLAN.md` (`cmd/customsvn`, `internal/...`, `web/`, `migrations/`).
- Match luatdo's style: comments explain why, errors are returned with context, tests need no network or credentials.
- UI text is Vietnamese by default through i18n keys; code, identifiers and commit messages are English.
- `prototypes/` is reference only. Ported code must reproduce the baseline numbers in `prototypes/README.md`; delete a prototype once it is ported.
- Data attribution lives in `NOTICE`. Any new data source gets an entry there before it is ingested.
