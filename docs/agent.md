# Agent and customs tools

Phase 3 of [PLAN.md](PLAN.md). The chat answers multi-step questions with tools, shows document status and amendment history as cards built from the data, asks back when a question is ambiguous, and is reachable from other programs over MCP and an OpenAI-compatible API.

## How a question flows

`internal/answer` (after the rewrite step of [chat.md](chat.md)):

1. **Route.** Rules on the words of the question come first: an HS code or "phân loại"/"mã HS" → `hs`; a document named with "còn hiệu lực" → `status`; a document named with "sửa đổi", "lịch sử", "hợp nhất" → `amendments`; "thủ tục", "hồ sơ", "trình tự" → `procedure`; otherwise `regulation`. When a `router` model is configured (a cheap one), it may move a `regulation` or `procedure` question to another intent, refuse it as `out_of_scope`, or ask back (`clarify`) when the answer depends on the import regime, the origin or agreement, or who imports (company, person, baggage, gift). The model never overrides an explicit HS or status question, and when it fails, the rules stand.
2. **Plan.** Code calls the tools the intent needs: `get_document_status`, `get_amendments`, and `get_provision` for every provision named in the question (up to three). An answer never looks up an HS code (owner's decision, 2026-10-29): a question about goods or a code is answered with the rules that apply to them and the provisions behind them; HS lines are for the HS screen, MCP and the API. These produce **cards**, which are shown as they are, and sources for the composer.
3. **Agent loop.** For `regulation`, `procedure` and `hs` questions, with every tool but `lookup_hs`, the answer model may call tools itself, up to 6 turns: search again with other wording, fetch a provision, check a status. Every call is a **step** in the trace the reader can unfold. A provider that cannot call tools (a luatdo routes file) just ends the loop.
4. **Compose and verify** as in Phase 2, over the named provisions first, then the search, then what the agent found (12 sources at most). The composer is told which cards the reader already sees, so it does not restate a status or a timeline in words it cannot cite.

An answer made only of cards (a status or amendments question) is a valid answer when a card found something and the composer neither refused nor wrote a claim that failed: the cards are data from the database, labelled "Từ dữ liệu", each with its source. Otherwise the question asked more than the cards say, and the reader sees the refusal beside them. The composer may end with up to three `gap` lines naming parts of the question no source answers; they carry no citation, state no rule, and show as "Chưa tìm được căn cứ".

The as-of day can be set per conversation (the date beside the input box). Every tool judges force on that day, by the dates rather than today's label: NĐ 128/2020/NĐ-CP is "không còn hiệu lực" today and "còn hiệu lực" as of 2024-01-01.

## Tools

`internal/tools`, shared by the agent, MCP and `POST /api/tools/{name}`. None of them calls a model.

| Tool | Input | Output |
| --- | --- | --- |
| `search_legal` | question | chunks in force, citable |
| `get_provision` | "khoản 2 Điều 18 Luật Hải quan" | the provision's chunk, consolidated wording first |
| `resolve_reference` | text | documents and provisions it names |
| `get_document_status` | number, law name or id | force on the as-of day, consolidated text, what replaced it |
| `get_amendments` | number, law name or id | timeline: issued, in force, amended, consolidated, replaced, ended |
| `lookup_hs` | code or description | HS lines with their parent chain and children, Vietnamese and English |
| `graph_query` | `amended_by`, `amends`, `detailed_by`, `details`, `cited_by`, `cites`, `replaced_by`, `consolidated_in` | related documents |

Status and relations read two tables that ingest now fills:

- `registry`: every one of the 128,238 store documents, in force or not, so a status question about an expired circular still has an answer.
- `edges`, with a normalised `relation`: vbpl.vn's official relation table (luatdo `parse.Th1nhng0Relations`, 716 thousand edges); links stated by document titles ("Sửa đổi, bổ sung một số điều của Nghị định số 08/2015/NĐ-CP", 654 edges where vbpl.vn has none); and each consolidated text's "được sửa đổi, bổ sung bởi" list. NĐ 59/2018/NĐ-CP, which amended NĐ 08/2015/NĐ-CP, is missing from vbpl.vn's table and comes back from its title. A title that names what the document implements before the change verb ("hướng dẫn thi hành Nghị định 49/2022 sửa đổi Nghị định 209/2013") yields nothing, because the verb belongs to the other document.

HS lines are placed in the tree by their code (8703.23.61 under 8703.23 under 87.03). The dashes that mark depth in the printed nomenclature were lost for whole runs of lines in the source HTML, so they are used only for lines with no code. Description search is trigram similarity, fused with HS embeddings once `customsvn embed -target hs -model NAME` has run.

## MCP and the OpenAI-compatible API

- `customsvn mcp` serves MCP over stdin/stdout; `customsvn serve` serves it over streamable HTTP at `/mcp`. Tools: the seven above plus `ask`, which returns a verified answer with its grounds as text.
- `POST /v1/chat/completions` (and `GET /v1/models`) answers as the model `customsvn`, streamed or not; when streamed, each claim is sent once its quotes are verified, and the grounds come at the end.
- Both take `Authorization: Bearer <token>` with the token in `CUSTOMSVN_API_TOKEN`, or a logged-in session. Without either they are closed (Phase 4; see [admin.md](admin.md)).

The settings screen **Kết nối MCP và API** shows ready-to-paste configurations.

## Checked so far

Against a fake model that routes, calls tools over the OpenAI streaming wire and quotes its sources: each intent called its tools and showed its card; the ambiguous import question was asked back without calling the answer model; the out-of-scope question was refused; the agent loop searched again; the conversation's as-of day applied to follow-ups; `/v1` (stream and not) and MCP over HTTP (initialize, the 8 tools, a status call, `ask`) answered. `go test ./internal/answer ./internal/tools ./internal/corpus` covers routing, the router model's refusals and questions back, the loop, title links and the HS tree.

Not done yet: running the experts' gold set through retrieval and answers and setting the release thresholds. It needs the gold set and an API key.
