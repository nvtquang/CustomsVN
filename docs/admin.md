# Administration and product polish

Phase 4 of [PLAN.md](PLAN.md): accounts, limits and costs, background data jobs, defences against prompt injection, configuration export, answer evaluation, and the admin screens built on them.

## Accounts

Every screen and every `/api` call needs a login. The session is a random token in an `HttpOnly`, `SameSite=Lax` cookie, kept for 30 days of use; the database stores only its SHA-256. Passwords are hashed with PBKDF2-SHA256 (600,000 rounds, from the Go standard library).

- **First admin.** When the database has no user, `serve` creates one: email `CUSTOMSVN_ADMIN_EMAIL` (default `admin`), password `CUSTOMSVN_ADMIN_PASSWORD`. No password is ever generated: without one no admin is created and the log says how to add one (`customsvn user add -email admin -role admin -password P`). Conversations made before accounts existed go to this admin.
- **Roles.** An admin manages data, settings, users and reviews. A user asks, reads documents, looks up HS codes and sees only their own conversations; someone else's conversation answers 404, not 403.
- **Users screen** (`/admin/users`): create (with a generated password shown once), change role, disable, reset the password, delete. Disabling or resetting ends the user's sessions. The last active admin cannot be disabled, demoted or deleted.
- **Locked out:** `customsvn user passwd -email admin` sets a new password from the shell. `customsvn user list` and `customsvn user add` are there too.
- **Cross-site requests.** A state-changing request is refused when the browser marks it cross-site (`Sec-Fetch-Site`), or, without that header, when its `Origin` is not the host asked for (`Host` or `X-Forwarded-Host`). Behind a TLS proxy that does not send `X-Forwarded-Proto`, set `CUSTOMSVN_SECURE_COOKIE=1`.
- **Other programs.** `/v1` and `/mcp` accept the bearer token `CUSTOMSVN_API_TOKEN` or a logged-in session. Without either they are closed; before Phase 4 they were open when no token was set.

## Limits and cost

- **Logins:** 10 attempts a minute per address and per account.
- **Questions:** per user, a minute and a day (default 10 and 500; 0 means no limit), set on the users screen. `/v1` calls with the token share one bucket. Over the limit, the answer is `429` with `Retry-After`, and the chat says so.
- **Usage:** every model call is recorded in `usage` with its user, conversation, provider and model (the target that actually answered, after failover) and its cost at the prices set in **Quản lý model** (USD per million tokens, under Nâng cao). Questions from `/v1` are recorded as `api:*`, from MCP as `mcp:*`, and from evaluations as `eval:*`. The dashboard lists the models used without a price, so a cost of 0 is not read as free.

## Background jobs

**Dữ liệu** (`/admin/data`) runs the same work as the commands, in the server, one job at a time:

| Job | Does | Notes |
| --- | --- | --- |
| prep | `customsvn prep` with an as-of day and legacy on or off | about a minute; not stopped halfway, so an ingest queued behind it never reads half-written files |
| ingest | `customsvn ingest` | the server loads the new corpus once the ingest writes a new version; an ingest that changed nothing skips the reload |
| embed | `customsvn embed`, chunks or HS descriptions, optional limit | needs the model's key |

Jobs live in `ingest_jobs`, with the log (head and tail kept, 512 KB) and the last line as progress. A job left running by a stopped server is marked failed on the next start. A running job can be canceled; ingest stops at the next phase. An ingest rewrites the relation and registry tables in one transaction (about 2.5 minutes on the full corpus) and holds their locks, so status and amendment questions wait until it commits.

The same screen shows corpus versions, counts by force, tier, source and document type, embeddings per model against what there is to embed, and the coverage report of the mandatory list from the last prep.

## Prompt injection

The model is one line of defence and the code is the other:

1. **Fences.** The question goes in `<question>…</question>` and each chunk in `<source>…</source>`. Text inside either cannot open or close a block: such tags are defused before the prompt is built. Legal text never contains them, so quotes still verify. The composer, router, rewrite and agent prompts all say that the question, the sources and tool results are data.
2. **Verification.** A claim reaches the reader only if every quote is found verbatim in the chunk it cites, as before. A model that obeys "answer without citations", "quote the rules you were given" or "cite source 9" produces claims that are dropped.
3. **No links.** A claim with a Markdown link or image, HTML, `javascript:` or `data:` content, or a web address that is not written in its cited sources, is dropped (`link`). A refusal reason with any of these is replaced, and such suggestions are left out. The chat renders claims without links or images whatever they contain, so a claim can never make the browser fetch an address carrying the conversation.

`internal/answer/injection_test.go` is the attack suite: overriding the rules, leaking the system prompt, a quote planted in the question, an invented source, exfiltration through an image, a phishing link, HTML, breaking the output format, a question that closes its own block, and an instruction inside a source. `goldsets/injection.csv` holds the same attacks for a real model in `/lab/answers`: pure attacks must be refused, and an attack wrapped around a real question must still be answered from the law.

## Configuration export and import

**Xuất/nhập cấu hình** (`/settings/config`) downloads a JSON file with the retrieval settings, the LLM providers (name, kind, base URL, the name of the environment variable of the key, models), the roles, the prices and the limits. It never holds a key. Importing replaces the parts the file has; a provider with the same name keeps the key this server stores, and the result lists the providers that need a key typed.

## Answer evaluation

`/lab/answers` runs a gold set through the whole answer (route, tools, compose, verify) with the stored models or others picked for the run, and scores what the reader would see. No model judges:

| Metric | Meaning |
| --- | --- |
| Refusal accuracy | refused exactly when the gold set says to |
| False refusals / missed refusals | answerable questions refused; questions to refuse that were answered |
| Claims verified | verified claims over all claims the model wrote: faithfulness before the verifier steps in (what is shown is verified by construction) |
| Citation precision | citations of an expected provision over all citations |
| Expected cited | mean share of expected provisions some citation covers |
| Grounded | answers with at least one expected provision cited |
| First claim p50/p95, total p95 | latency as the reader sees it |
| Cost | at the stored prices |

Each question's answer is stored, so a run can be opened question by question (problems only by default), and two runs compared metric by metric with the questions whose outcome changed. The run's stored configuration is the one in effect, with the overrides merged in.

## Feedback review

`/admin/feedback` lists ratings by status (open, resolved, dismissed) and by 👍/👎. A feedback opens with the whole conversation (the rated answer framed), the trace (question as understood, intent, timing, dropped claims and why, retrieved chunks, model), a label (wrong citation, missing grounds, wrong refusal, should have refused, wrong force, incomplete, unclear, data gap, other) and a note. **Chuyển thành gold item** resolves the expected provisions against the corpus and adds the question to a gold set (by default `feedback`, which grows item by item), and marks the feedback resolved.

## Dashboard

`/admin`: questions, people asking, refusal rate, time to the first claim (p50 and p95 over the whole range), cost (readers and evaluations apart), feedback, and open 👎, for 7, 30 or 90 days counted in Asia/Ho_Chi_Minh; four daily charts, or the same figures as a table.

## Interface

- Login screen. The sidebar holds a new question, HS lookup, the lab (admins), settings, and the conversation history. The user bar at its foot opens a menu: settings, profile and password, administration (admins), shortcuts, logout.
- **Settings** take the whole window, sections on the left as in Dify and WeKnora, a close button back to the screen the user came from: General (theme, language, shortcuts), Profile and password; for admins Models, Knowledge and retrieval, and the administration screens (dashboard, feedback, data, users); MCP and API, configuration export. On a phone the sections scroll sideways above the page. The old addresses (`/settings/llm`, `/settings/retrieval`, `/account`, `/help/integrations`) redirect.
- Shortcuts: Ctrl+K (⌘K) new question, `/` to the question box, Enter to send, Shift+Enter for a new line, Esc to stop an answer and close the panel or menu, `?` to list them.
- Accessibility: a skip link, labels tied to their controls, table headers marked as such, the streaming answer in a polite live region, dialogs and menus with roles and labels.
- Phone width: the sidebar is a drawer, tables scroll inside their cards, and no page scrolls sideways (tested).

## Models

**Quản lý model** (`/settings/models`) lists every model by type: Chat, Embedding, Rerank, and Vision and Speech, which no feature uses yet and so cannot be added. **Thêm model** asks for the type, the source (an API, or Ollama on the machine), the provider and its key, and the model name, offering the provider's own list or known names, and **Kiểm tra kết nối** tries the settings as typed before anything is saved: a one-word chat, an embedding of one sentence (which fills in the vector size), or a rerank of a relevant and an unrelated passage.

| Provider | Chat | Embedding | Rerank |
| --- | --- | --- | --- |
| Anthropic | yes | | |
| OpenAI, Gemini, DeepSeek, any OpenAI-compatible API | yes | OpenAI, Gemini, compatible | only at a base URL of its own serving `/rerank` (TEI, Xinference) |
| OpenRouter | yes | yes | yes |
| Voyage, Jina | | yes | yes |
| Cohere | | | yes |
| Ollama | yes | yes | |

## System prompts

**System prompt** (`/settings/prompts`, admins) holds the instructions of each model call of an answer: writing the answer, the research agent, the question classifier and the rewriting of follow-ups. Each is filled with the default and can be edited and saved; the next question uses it. A prompt saved equal to its default is stored empty, so a better default in a later release reaches installs that never changed it. The output format and the rules that keep sources and questions from acting as instructions are appended by code and shown read-only under each prompt: an edit changes how answers read, not whether they parse, and every quote is still verified verbatim. The prompts are part of the configuration export.

## Scheduled update

The Data screen schedules the crawl of vbpl.vn (monthly or weekly, day, hour, catalogue pages; the 1st of the month at 02:00 by default) and runs it at once on demand. See [crawl.md](crawl.md).

An embedding model or reranker of the settings screen is named `provider/model`; its vectors are stored under that name. They are added to those of `customsvn.yaml`, which show on the screen as read-only. Saving rebuilds the router, the embedders and the rerankers at once, without a restart. The chat model for each task (answer, question classifier, follow-ups), with a fallback, is picked on the same screen; the embedding model and the reranker on the knowledge screen.

## Knowledge and retrieval

**Kiến thức và truy xuất** (`/settings/knowledge`):

- **Chunking.** *General*: an article per chunk, split at its clauses past a length (default 4,000 characters, as before). *Parent-child*: every clause its own chunk, the child a search matches; the rest of its article, the parent, comes after it for the model to read, nearest clauses first, up to a length (default 6,000). The separators are not a setting: chunks follow the article and clause structure so every citation names a provision. Chunking is applied by prep (`customsvn prep -chunking parent_child`, or the prep job, which reads the setting); the page says how the data in use was cut and offers to run prep and ingest again. Each chunk added as parent context is a real chunk, so its quotes verify as usual.
- **Index method.** *High quality*: an embedding model beside keywords, with how many chunks have its vectors and a button that queues the embed job for the rest. *Economy*: keywords only, no API calls; answers search BM25 whatever the mode says.
- **Retrieval.** Vector, full-text or hybrid search (with the keyword/meaning balance), rerank with its model, **Top K** (passages an answer is given, default 8) and a **score threshold**. The threshold applies to rerank scores, or to cosine similarity in a vector search; fused and BM25 scores have no 0–1 scale, so it is not offered then. A provision named in the question is never dropped.
- Scope (document groups, versions) and the algorithm's numbers stay below, folded.

With the defaults (general chunking at 4,000, top K 8, no threshold, no parent context) retrieval and answers are what they were before these settings existed. Parent-child, a threshold or another top K change what the model reads: measure them in Retrieval Testing and answer evaluation before keeping them.

## Tests

- Go: `internal/auth` (hashing, validation), `internal/ratelimit`, `internal/jobs` (log and progress), `internal/evalx` (answer scoring), `internal/answer` (the attack suite).
- Playwright, in `web/e2e`, against a running test server with a loaded corpus and a fake model: login and logout, a user kept out of admin screens and APIs, a verified answer with its source panel, a 👎 reviewed and turned into a gold item, a refusal, the shortcuts, history, the dashboard, a job queued and canceled with its log, user creation, a config export without keys, an answer evaluation, and the phone layout. 13 tests pass against the embedded UI. See [web/e2e/README.md](../web/e2e/README.md).

Still open from the plan: an expert week on the beta with every 👎 handled in the review screen, and the thresholds that need real API keys and the experts' gold set.
