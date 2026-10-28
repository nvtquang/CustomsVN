# Chat: answers with verified citations

Phase 2 of [PLAN.md](PLAN.md). A question gets an answer whose every claim carries a quote found word for word in a chunk of a document in force on the as-of day, or it gets a refusal.

## Running it

```sh
cp .env.example .env                  # set CUSTOMSVN_MASTER_KEY: openssl rand -base64 32
docker compose up -d --build          # app + PostgreSQL 17 + pgvector
docker compose run --rm app bootstrap # first data load (luatdo fetch/parse/cite/subjects, prep, ingest)
```

Then open http://localhost:8080 and add a model in **Cài đặt → Quản lý model** (the user bar at the foot of the sidebar). Without a configured answer model the chat screen says so and links to the settings; Retrieval Testing works without one.

`bootstrap` skips every step whose output already exists, so it can be rerun after an interruption. To reuse an existing luatdo data directory, mount it at `/data` instead of the `customsvn-data` volume.

## How an answer is made

`internal/answer`:

1. **Rewrite.** A follow-up ("còn thời hạn thì sao?") is rewritten into a question that stands alone, by the `rewrite` model (or the `answer` model when no rewrite model is set), from the last six turns.
2. **Deterministic refusals.** A question asking for the duty rate of a specific code is refused before any model is called: the corpus has no tariff schedules, and a rate guessed from a decree reads as right when it is wrong.
3. **Retrieve.** The hybrid search of Phase 1 with the stored retrieval settings, top 8, scoped to documents in force on the as-of day.
4. **Compose.** The `answer` model gets the chunks as numbered sources, each with its document, heading path and force, and returns JSON Lines: one claim per line, each with `{"source": n, "quote": "..."}` citations, or a single refusal line. The system prompt says the sources are data, so an instruction inside a source is quoted, never followed.
5. **Verify, claim by claim.** As soon as a line is complete, every quote is checked against the cited chunk (`answer.Verbatim`: Unicode NFC, quotes and dashes unified, white space collapsed, nothing else forgiven; an ellipsis may join pieces of at least 12 characters that appear in order) and the cited document's force is checked again. A claim with any failing quote is dropped and counted. A verified claim is sent to the reader at once.
6. **No claim left means a refusal**, with suggestions for asking again.

The answer therefore streams claim by claim, not token by token. The first verified claim is the moment the reader first sees an answer, and the response records it as `first_claim_ms`.

Citation markers `[n]` are added from the verified citations only; any marker the model writes into a claim is removed. Each answer is stored with its claims, its verified citations (`citations` table), the dropped claims and the retrieval trace, and the LLM usage goes to `usage`.

## Models

`internal/llm` has its own Anthropic Messages and OpenAI-compatible `/chat/completions` wires, with streaming and tool calls, plus an adapter that uses a luatdo routes file as one provider. luatdo's completers return whole texts, so a luatdo provider answers in one piece and cannot call tools. Each role (`answer`, `rewrite`) has an ordered list of targets. A target that fails before its first token fails over to the next one. A failed provider cools down for 24 hours on an auth failure, a minute on a quota failure, and 10 seconds on a server or network failure.

Providers and roles live in the database and are edited in **Quản lý model**. A key typed there is encrypted with AES-256-GCM under `CUSTOMSVN_MASTER_KEY` and never returned to the browser, which only sees a hint such as `sk-…1234`. A provider can instead name an environment variable. Without a master key, only environment variables work.

## API

| Endpoint | Purpose |
| --- | --- |
| `POST /api/chat` | Server-sent events: `meta`, `status`, `rewrite`, `intent`, `step`, `card`, `clarify`, `claim`, `dropped`, `refusal`, `done`, `error` (Phase 3 added intent, step, card, clarify; see [agent.md](agent.md)). Body: `question`, `conversation_id`, `as_of`, `regenerate` |
| `GET /api/conversations?q=` | History, newest first, searchable |
| `GET/PATCH/DELETE /api/conversations/{id}` | One conversation with its messages, claims and ratings; rename; delete |
| `POST /api/feedback` | 👍/👎 with a reason, one rating per answer |
| `GET /api/documents/{id}` | A document with its structure tree, today's force and its consolidation links |
| `GET /api/provisions/{id}` | One provision |
| `GET/PUT /api/settings/llm`, `POST /api/settings/llm/test` | Providers, roles, connection test |

Every endpoint needs a login since Phase 4, and a user sees only their own conversations ([admin.md](admin.md)).

An `error` event's `kind` is `no_model`, `auth`, `quota`, `server`, `network`, `request` or `canceled`, and the UI says each one differently. When the reader stops an answer, the claims already shown are stored.

## Checked so far

There is no API key on the development machine yet, so the pipeline was checked end to end against a fake OpenAI-compatible model that quotes its sources and adds one invented claim. Verified claims arrived 0.49 s after the question; the invented claim was dropped. Follow-up rewriting, regenerate, the tariff refusal, history, rename, search, feedback, the document viewer scrolled to and highlighting the quote, and storing a key encrypted with only its hint returned all worked. `go test ./internal/answer ./internal/llm` covers the same paths with scripted models.

Still to measure with a real model: the share of answers with at least one valid citation, the time to first claim at p95 (target under 3 s), and refusals on out-of-scope questions. That needs a key and an answer evaluation, which is Phase 4's `/lab/answers`.
