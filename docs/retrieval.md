# Retrieval and Retrieval Testing

Phase 1 of [PLAN.md](PLAN.md): measure search quality before there is a chatbot.

## Running it

```sh
docker compose -f deploy/docker-compose.yml up -d          # PostgreSQL 17 + pgvector on :55432
export CUSTOMSVN_DATABASE_URL='postgres://customsvn:customsvn@localhost:55432/customsvn?sslmode=disable'
export LUATDO_DATA=/path/to/luatdo

customsvn prep -as-of 2026-10-27        # build the corpus (docs/corpus.md)
customsvn ingest                        # load it; migrates the schema first
customsvn embed -model openai-3-large-1024   # optional, needs the key named in customsvn.yaml
customsvn eval -goldset goldsets/provisional.csv
customsvn serve                         # API and Web UI on 127.0.0.1:8080
```

Every command that touches the database applies the migrations in `migrations/` first. Copy `customsvn.example.yaml` to `customsvn.yaml` to configure embedding models and a reranker. The file holds no keys, only the names of the environment variables that do.

## Ingest

`ingest` compares each document and chunk with the database by content hash and writes only what changed. A run that changes anything records a new `corpus_version`; a run that changes nothing records none, so a retrieval run's version names exactly the data it measured. The provision trees go into `provisions`, luatdo's citation and amendment links plus each VBHN's link to its base go into `edges`, and the HS nomenclature goes into `hs_codes`. The first ingest of the 2026-10-27 corpus took about a minute, and a repeat took 13 seconds and changed nothing.

## Search

`internal/search` runs five steps and reports each one:

1. **References.** `internal/refs` finds "điểm b khoản 1 Điều 18 Luật Hải quan", official numbers, HS codes and dates. A provision whose instrument is named, by number or by the longest known law name the text starts with, is looked up and put first.
2. **Scope.** Tiers (1 and 2 by default), document types, source kinds, and force on the as-of day. Force comes from the dates: a document is in force from its effective date until its expiry date. For today the source's status label also counts, but never on its own. A consolidated text has the force of its base. The scope is cut before anything is ranked.
3. **Rankers.** luatdo's BM25 over each chunk's text and heading path (top 50); pgvector cosine search for the chosen embedding model (top 50, HNSW with iterative scan so the scope filter does not shorten the list); and a third BM25 list restricted to instruments the question names by number.
4. **Fusion.** Reciprocal rank fusion (k = 60, a weight per list), then an optional reranker over the top 30.
5. **Consolidated wording.** An original chunk whose document has a VBHN is replaced by the VBHN chunk of the same provision, and duplicates are folded.

Hybrid search without a configured embedding model runs BM25 alone and says so in the response notes.

## Metrics

`internal/evalx` scores each question against its expected provisions. A hit counts when the chunk is the expected provision, contains it (an article contains its clauses), or is part of it (a clause of a split article). A consolidated chunk counts for the base provision it consolidates.

- recall@1/3/5/10: the share of expected provisions in the top k, averaged over questions
- hit@10: the share of questions with at least one expected provision in the top 10
- MRR and nDCG@10, with binary relevance
- expired rate: the share of top-10 hits not in force on the question's as-of day
- latency p50 and p95 per question

Questions marked `should_refuse` are counted but left out of the ranking metrics; refusal is scored with answers in Phase 2.

## Gold sets

The format is the one in PLAN.md: CSV or JSON with `id, question, expected_provisions, expected_answer, as_of_date, category, should_refuse, notes`. Expected provisions are written as text ("khoản 1 Điều 18 Luật Hải quan 54/2014/QH13", several separated by `;`) and resolved to identifiers when the set is imported. Every unresolvable row is reported at once. Importing under an existing name adds a version.

`goldsets/provisional.csv` is a stopgap until the experts' set arrives: 45 questions written against the article headings of ten core instruments (41 answerable, 4 to refuse). It measures the system on questions phrased by a developer, so its numbers are a floor to improve on, not a quality claim.

## Baseline

Corpus version 1 (2026-10-27, without `legacy_content`), provisional gold set, BM25 only (no embedding key on the development machine):

| Configuration | R@1 | R@3 | R@5 | R@10 | Hit@10 | MRR | nDCG@10 | Expired top-10 | p95 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Default (hybrid → BM25) | 0.378 | 0.646 | 0.671 | **0.805** | 0.829 | 0.576 | 0.615 | **0.000** | 244 ms |
| `-keep-originals` | 0.378 | 0.646 | 0.671 | 0.805 | 0.829 | 0.576 | 0.615 | 0.000 | 365 ms |
| `-ignore-refs` | 0.354 | 0.622 | 0.646 | 0.780 | 0.805 | 0.551 | 0.590 | 0.000 | 416 ms |

Latency differs between runs on the same machine; compare it only within one session.

Corpus version 2 adds the 100 consolidated texts recovered from `legacy_content` (now the default):

| Corpus | R@1 | R@3 | R@5 | R@10 | Hit@10 | MRR | nDCG@10 | Expired top-10 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| v1, without legacy | 0.378 | 0.646 | 0.671 | 0.805 | 0.829 | 0.576 | 0.615 | 0.000 |
| v2, with legacy | 0.402 | 0.646 | 0.683 | 0.793 | 0.805 | 0.585 | 0.621 | 0.000 |

The one new miss, t14 ("which goods are exempt from import duty"), now ranks the consolidated decree 134/2016/NĐ-CP (11/VBHN-BTC), whose articles list the exemptions in detail, above Article 16 of the law the gold set expects. That is a gold-set question as much as a retrieval one; the expert set should say which level of instrument it expects.

Seven questions miss the top 10: t05 (release before clearance), t07 (inspection in the declarant's absence), t13 (duty payment deadline), t27 (wholly obtained goods), t30 (when to submit a C/O), t35 (late declaration penalty) and t39 (who is a declarant). Most are phrased away from the wording of the article. The embedding benchmark is the next step, and it needs an API key for each provider in `customsvn.example.yaml`.

Preferring consolidated wording changes nothing on this set, because without `-legacy` the core instruments in it have no VBHN in the corpus.

## API

| Endpoint | Purpose |
| --- | --- |
| `GET /healthz` | Server, database and corpus version |
| `GET /api/meta` | Corpus counts, models, defaults |
| `POST /api/retrieval/test` | One search, with every score and the scope arithmetic |
| `GET/PUT /api/settings/retrieval` | Default filters and configuration |
| `GET /api/chunks/{id}` | A chunk with its document and today's force |
| `POST /api/goldsets` | Import a gold set (`dry_run` previews the row errors) |
| `GET /api/goldsets`, `GET /api/goldsets/{id}` | Stored sets |
| `POST /api/retrieval/runs` | Start a batch run in the background |
| `GET /api/retrieval/runs`, `GET /api/retrieval/runs/{id}` | History, progress, metrics, per-question results |
