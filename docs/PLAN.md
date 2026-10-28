# Kế hoạch phát triển CustomsVN

Bản gốc: https://claude.ai/artifact/3zoweSkudG7yqaqPKwzV3b (2026-10-27). Từ nay file này là bản chính. Mỗi khi hoàn thành một mục, đánh dấu checkbox ở đây.

## Tóm tắt

CustomsVN là chatbot kiêm agent AI tra cứu pháp luật hải quan, mã nguồn mở theo giấy phép MIT. Backend viết bằng Go và kế thừa luatdo; Web UI viết bằng React; dữ liệu nằm trong PostgreSQL + pgvector; LLM gọi qua dịch vụ cloud. Bản v1.0 dự kiến xong sau khoảng 19 tuần, qua 6 phase. Mọi câu trả lời phải trích dẫn tới Điều, Khoản, Điểm cụ thể kèm hiệu lực của văn bản; không đủ căn cứ thì từ chối trả lời.

| Hạng mục | Quyết định | Ghi chú |
| --- | --- | --- |
| Repo | Repo mới `CustomsVN`, import luatdo làm Go module, pin theo commit; không gửi PR lên luatdo, lỗi của luatdo xử lý ngay trong CustomsVN | Binary tên `customsvn` |
| Giao diện | Tự xây Web UI bằng React | Bản build nhúng vào binary Go |
| CSDL | PostgreSQL 17 + pgvector | |
| LLM | Dịch vụ cloud qua API key (Claude, các endpoint tương thích OpenAI) | LLM local bổ sung ở Phase 6 |
| Giấy phép | MIT | Đồng bộ với luatdo |
| Gold set | Chuyên gia hải quan soạn, gửi sau | Retrieval Testing có trong UI từ Phase 1 |
| Crawl vbpl.vn | Làm sau v1.0 | Phase 6 |

## Mục tiêu, phạm vi và nguyên tắc

Mục tiêu v1.0: người làm xuất nhập khẩu hỏi bằng tiếng Việt về thủ tục, chính sách, thuế XNK, mã HS hoặc trạng thái văn bản, và nhận câu trả lời có trích dẫn kiểm chứng được.

| | Trong phạm vi v1.0 | Ngoài phạm vi v1.0 |
| --- | --- | --- |
| Dữ liệu | Văn bản hải quan, XNK, thuế và lĩnh vực liên quan còn hiệu lực; VBHN; Danh mục HS (TT 31/2022) | Công văn hướng dẫn, bản án, cập nhật tự động bằng crawl |
| Chức năng | Hỏi đáp có trích dẫn, tra HS, tra trạng thái và chuỗi sửa đổi, trả lời theo mốc ngày | Tính số thuế phải nộp, tư vấn pháp lý có trách nhiệm |
| Giao diện | Chat, trình xem văn bản, tra HS, Retrieval Testing, cài đặt, quản trị dữ liệu và feedback | Ứng dụng mobile native |
| Tích hợp | REST API, MCP server, endpoint tương thích OpenAI | SSO doanh nghiệp |
| Mô hình | LLM cloud | LLM local (Phase 6) |

Nguyên tắc thiết kế:

1. Không có căn cứ thì không trả lời. Mỗi nhận định phải có câu trích nguyên văn từ điều khoản, kiểm tra tự động trước khi hiển thị.
2. Không bao giờ dùng văn bản đã hết hiệu lực làm căn cứ cho quy định hiện hành.
3. Có văn bản hợp nhất thì ưu tiên dùng thay cho bản gốc.
4. Phần tất định làm trước, LLM dùng sau: cấu trúc, ID, hiệu lực, quan hệ sửa đổi đều lấy bằng code.
5. Mọi thay đổi về truy xuất phải chạy qua Retrieval Testing, có số đo trước và sau.
6. File cấu hình không chứa secret; API key lưu mã hóa và không bao giờ gửi về trình duyệt.
7. Cài đặt chỉ bằng một lệnh, chạy được trên Windows, Linux và macOS.

## Hiện trạng (2026-10-27)

| Tài sản | Hiện trạng | Dùng ở đâu |
| --- | --- | --- |
| luatdo: `parse`, `law` | Parser tất định, ID ổn định tới từng Điểm, cơ chế cách ly; đã dùng lại được cho VBHN | Pipeline dữ liệu |
| luatdo: `cite` | 762.965 trích dẫn và quan hệ sửa đổi | Bảng `edges`, công cụ tra chuỗi sửa đổi |
| luatdo: `subject` | Có sẵn `thue/hai-quan`, `thue/thue-xuat-nhap-khau`, `thuong-mai/xuat-nhap-khau` | Phân tầng |
| luatdo: `retrieve` | BM25 đa trường, lọc theo ngày hiệu lực, loại trùng lặp | Nhánh BM25 của hybrid search |
| luatdo: `route`, `api` | Định tuyến nhiều nhà cung cấp LLM, failover, streaming | Lớp LLM |
| luatdo: `eval` | Khung đo chất lượng | Retrieval Testing |
| Corpus hải quan | 70.283 chunk: 13.376 tầng 1, 56.907 tầng 2 | Nạp vào Postgres |
| VBHN | 38 văn bản, 1.911 chunk, có văn bản gốc, chuỗi sửa đổi, chú thích từng khoản | Ưu tiên khi truy xuất |
| Văn bản cứu từ diện cách ly | 379 văn bản, có NĐ 66/2024 và TT 01/2024/TT-BNNPTNT | Corpus |
| Danh mục HS | 18.157 dòng, 11.414 mã 8 số, có cây cha con, song ngữ | `hs_codes`, công cụ tra HS |
| Prototype | `prototypes/` (Python và `hqprep`) | Chuyển sang Go ở Phase 0 |

Khoảng trống dữ liệu:

- Nguồn không có biểu thuế XNK của NĐ 26/2023 và các bản sửa đổi; trên vbpl.vn phụ lục này là file đính kèm.
- 1.902 trên 2.242 VBHN chỉ là trang HTML rỗng.
- Khoảng 360 văn bản ban hành 1995–2009 vẫn ghi "Còn hiệu lực" nhưng cần chuyên gia xác minh.
- Snapshot dữ liệu chụp ngày 23/07/2026; một số văn bản mới (ví dụ TT 121/2025/TT-BTC) chỉ có metadata.
- Chưa có công văn hướng dẫn và chưa có embedding.

## Kiến trúc tổng thể

```text
 Web UI (React 19)                    Công cụ ngoài (Claude Desktop, Dify, Open WebUI)
        │ REST + SSE                          │ MCP · API tương thích OpenAI
┌───────▼─────────────── customsvn · một binary Go ───▼──────────────────────┐
│ HTTP API: REST + SSE · MCP server · /v1/chat/completions · UI qua go:embed │
│  Search ◄── Tools ◄── Agent ──► LLM                                        │
│  BM25 (luatdo)  search_legal   router ý định    route (luatdo)             │
│  pgvector+RRF   get_provision  vòng lặp ≤ 6     Anthropic                  │
│  rerank, lọc    lookup_hs      composer+verifier OpenAI-compatible          │
└──────┬───────────────┬─────────────────────────────────┬───────────────────┘
       ▼               ▼                                 ▼
 PostgreSQL 17 + pgvector                         LLM, embedding API (cloud)
 chunk, vector, đồ thị sửa đổi, mã HS, hội thoại, gold set, kết quả test
       ▲ nạp theo hash
 Pipeline dữ liệu (CLI): luatdo fetch, parse, cite, subjects → customsvn prep → ingest → embed
       ▲
 Nguồn: vbpl.vn qua th1nhng0, UTS_VLC, VBHN
```

## Kiến trúc backend Go

```text
CustomsVN/
  cmd/customsvn/      lệnh: prep, ingest, embed, serve, eval, mcp
  internal/
    corpus/           phân tầng, VBHN, cứu cách ly, bảng HTML, danh mục HS
    db/               PostgreSQL (pgx), repository, migration
    embed/            interface embedding + các provider
    search/           BM25 (luatdo retrieve) + pgvector + RRF + rerank, bộ lọc
    refs/             nhận diện "khoản 2 Điều 18 Luật Hải quan", mã HS, ngày
    llm/              bọc luatdo route + wire Anthropic + tool calling + streaming
    agent/            intent router, vòng lặp tool, composer, verifier
    tools/            các công cụ của agent
    httpapi/          REST + SSE, OpenAPI, endpoint tương thích OpenAI
    mcpserver/        MCP server (stdio và HTTP)
    config/           cấu hình YAML + env, mã hóa API key
    evalx/            Retrieval Testing, đánh giá câu trả lời
    auth/             tài khoản, phiên, vai trò (Phase 4)
  migrations/         SQL migration (goose)
  web/                ứng dụng React, build nhúng bằng go:embed
  deploy/             Dockerfile, docker-compose.yml
  docs/               PLAN.md, kiến trúc, cài đặt, đóng góp
  prototypes/         script thử nghiệm từ giai đoạn khảo sát, xóa sau khi port xong
```

| Endpoint | Chức năng | Phase |
| --- | --- | --- |
| `GET /healthz` | Trạng thái server và CSDL | 0 |
| `POST /api/retrieval/test` | Chạy một truy vấn, trả kết quả kèm điểm từng thành phần | 1 |
| `POST /api/goldsets`, `POST /api/retrieval/runs`, `GET /api/retrieval/runs/{id}` | Nhập gold set, chạy hàng loạt, xem kết quả | 1 |
| `GET/PUT /api/settings/retrieval` | Tham số truy xuất | 1 |
| `POST /api/chat` (SSE) | Hỏi đáp streaming: token, trích dẫn, các bước agent | 2 |
| `GET /api/conversations`, `GET /api/conversations/{id}` | Lịch sử hội thoại | 2 |
| `GET /api/documents/{id}`, `GET /api/provisions/{id}` | Văn bản, điều khoản, hiệu lực, chuỗi sửa đổi | 2 |
| `GET/PUT /api/settings/llm`, `POST /api/settings/llm/test` | Provider, model, API key, kiểm tra kết nối | 2 |
| `POST /api/feedback` | Đánh giá câu trả lời | 2 |
| `GET /api/hs?q=`, `GET /api/hs/{code}` | Tra mã HS | 3 |
| `POST /v1/chat/completions`, `/mcp` | Tích hợp công cụ ngoài | 3 |
| `/api/admin/*` | Corpus, job nạp dữ liệu, feedback, người dùng, thống kê | 4 |

## Kiến trúc dữ liệu

Một CSDL PostgreSQL 17 + pgvector chứa toàn bộ trạng thái. Mỗi lần nạp dữ liệu tạo một `corpus_version`, nhờ vậy mỗi lần chạy Retrieval Testing biết mình đo trên bộ dữ liệu nào. Neo4j chỉ dùng để khám phá, không cần khi chạy.

| Bảng | Nội dung | Index, ghi chú |
| --- | --- | --- |
| `documents` | Số hiệu, loại, cơ quan, ngày ký, ngày hiệu lực, ngày hết hiệu lực, trạng thái, tầng, nguồn, `consolidated_in` | Số hiệu chuẩn hóa, tầng, trạng thái |
| `provisions` | Cây Chương/Điều/Khoản/Điểm, text, `text_hash` | `document_id`, ID cha |
| `chunks` | Text, đường dẫn tiêu đề, tầng, hiệu lực, `source_kind`, chú thích sửa đổi | GIN cho bộ lọc, `corpus_version` |
| `chunk_embeddings` | `vector(n)` theo từng `embedding_model` | HNSW cosine; nhiều model cùng tồn tại để so sánh |
| `edges` | Sửa đổi, trích dẫn, hợp nhất, thay thế | Recursive CTE |
| `hs_codes` | Mã, cấp, mô tả Việt và Anh, đơn vị, mã cha, chương, văn bản nguồn | Theo mã; trigram cho mô tả |
| `tariffs` | Thuế suất theo mã HS, biểu thuế, khoảng hiệu lực | Schema từ Phase 0, nạp khi có nguồn |
| `conversations`, `messages`, `citations` | Hội thoại, tin nhắn, trích dẫn kèm kết quả kiểm chứng | Theo người dùng và thời gian |
| `feedback` | 👍/👎, lý do, nhãn review | Liên kết `messages` |
| `gold_sets`, `gold_items` | Câu hỏi chuẩn và điều khoản kỳ vọng | Có phiên bản |
| `retrieval_runs`, `retrieval_results` | Cấu hình, `corpus_version`, metric, kết quả từng câu | So sánh các lần chạy |
| `settings`, `llm_providers` | Cấu hình; API key mã hóa AES-GCM bằng master key từ env | Không trả key về client |
| `users`, `ingest_jobs`, `usage` | Tài khoản, job nền, token và chi phí | Phase 4 |

Pipeline (chạy lại luôn ra cùng kết quả):

1. `luatdo fetch`, `parse`, `cite`, `subjects`: snapshot đã pin, parse, nối trích dẫn.
2. `customsvn prep`: phân tầng, cứu văn bản bị cách ly, VBHN, tách bảng HTML, danh mục HS, cắt chunk.
3. `customsvn ingest`: ghi vào Postgres theo hash, chỉ ghi phần thay đổi, sinh `corpus_version` mới.
4. `customsvn embed`: chỉ tính embedding cho chunk mới hoặc đã đổi, theo từng model.
5. `customsvn serve` khi khởi động: nạp chỉ mục BM25 vào RAM (khoảng 70 nghìn chunk); HNSW nằm sẵn trong Postgres.

## Kiến trúc truy xuất và agent

Các bước truy xuất:

1. Phân tích câu hỏi: chuẩn hóa; nhận diện tham chiếu, mã HS, mốc ngày. Có tham chiếu thì tra thẳng điều khoản đó.
2. Lọc phạm vi: tầng 1 và 2, còn hiệu lực tại mốc ngày (mặc định hôm nay), loại văn bản.
3. Chạy song song BM25 (luatdo `retrieve`, top 50) và vector (pgvector, top 50).
4. Gộp bằng RRF, rerank nếu bật, lấy top 10.
5. Ưu tiên VBHN: chunk bản gốc có `consolidated_in` được thay bằng chunk tương ứng trong VBHN; loại trùng lặp.
6. Gửi ngữ cảnh cho LLM kèm ID chunk, đường dẫn tiêu đề, hiệu lực.

Agent:

- **Intent router** dùng model rẻ: quy định, thủ tục, mã HS, trạng thái văn bản, sửa đổi theo thời gian, ngoài phạm vi.
- **Vòng lặp tool** tối đa 6 bước, lưu trace từng bước.
- **Composer** trả JSON: danh sách nhận định, mỗi nhận định kèm `chunk_id` và câu trích nguyên văn.
- **Verifier** kiểm câu trích nguyên văn và hiệu lực tại mốc ngày; bỏ nhận định không đạt; không còn nhận định nào thì từ chối và gợi ý cách hỏi lại.

| Công cụ | Đầu vào | Đầu ra | Phase |
| --- | --- | --- | --- |
| `search_legal` | Câu hỏi, bộ lọc | Chunk có điểm | 2 |
| `get_provision` | ID hoặc tham chiếu dạng chữ | Text, đường dẫn, hiệu lực, chú thích sửa đổi | 2 |
| `resolve_reference` | "điểm b khoản 1 Điều 18 Luật Hải quan" | ID điều khoản | 3 |
| `get_document_status` | Số hiệu hoặc ID | Trạng thái, hiệu lực, VBHN, văn bản thay thế | 3 |
| `get_amendments` | ID văn bản | Chuỗi sửa đổi theo thời gian | 3 |
| `lookup_hs` | Mã hoặc mô tả | Mã, cây cha con, chú giải, văn bản nguồn | 3 |
| `graph_query` | Câu hỏi định sẵn về đồ thị | Bảng kết quả | 3 |

Model mặc định, đổi được trong UI: trả lời `claude-sonnet-5-5`; router và viết lại câu hỏi `claude-haiku-5-5`; mọi endpoint tương thích OpenAI đều thay vào được.

## Kiến trúc Web UI

Ứng dụng React một trang, build bằng Vite, nhúng vào binary. Màn hình đầu tiên là Retrieval Testing (Phase 1), sau đó mới đến Chat.

| Màn hình | Đường dẫn | Chức năng | Phase |
| --- | --- | --- | --- |
| Retrieval Testing | `/lab/retrieval` | Thử truy vấn với bộ lọc; điểm BM25, vector, rerank; so sánh A/B; nhập gold set, chạy hàng loạt, metric, câu trượt | 1 |
| Cài đặt truy xuất | `/settings/retrieval` | top-k, trọng số RRF, rerank, tầng, model embedding | 1 |
| Chat | `/` | Streaming, trích dẫn [n], dừng/tạo lại, gợi ý câu hỏi, 👍/👎, miễn trừ | 2 |
| Panel trích dẫn | Panel phải của Chat | Text điều khoản, đường dẫn, nhãn hiệu lực, "đã hợp nhất trong", chú thích sửa đổi, link vbpl.vn | 2 |
| Trình xem văn bản | `/doc/:id` | Cây cấu trúc, highlight đoạn trích, trạng thái, văn bản sửa đổi | 2 |
| Cài đặt LLM | `/settings/llm` | Provider và model theo vai trò, API key (che), test kết nối | 2 |
| Tra cứu HS | `/hs` | Tìm theo mã/mô tả, cây phân cấp, chú giải chương, song ngữ | 3 |
| Quản trị dữ liệu | `/admin/corpus` | Phiên bản corpus, thống kê, độ phủ, chạy ingest/embed, log | 4 |
| Feedback và đánh giá | `/admin/feedback`, `/lab/answers` | Duyệt 👎, gắn nhãn, chuyển thành gold item; đánh giá end-to-end; dashboard | 4 |

Bố cục: sidebar trái (lịch sử, menu), vùng giữa (nội dung), panel phải (trích dẫn, chi tiết); trên màn hình hẹp panel phải thành ngăn kéo. Có sáng/tối; tiếng Việt mặc định, sẵn khung cho tiếng Anh.

```text
web/src/
  routes/        màn hình (TanStack Router, định tuyến theo file)
  features/      chat/, citations/, documents/, hs/, retrieval-lab/, settings/, admin/
  components/ui/ thành phần giao diện cơ bản (shadcn/ui)
  lib/api/       client sinh từ OpenAPI, đọc SSE
  lib/i18n/      vi, en
  stores/        trạng thái phía client (Zustand)
```

## Tech stack

| Lớp | Công nghệ | Ghi chú |
| --- | --- | --- |
| Backend | Go ≥ 1.26.5 (dev dùng 1.27) | Theo yêu cầu của luatdo |
| Thư viện kế thừa | luatdo: `parse`, `law`, `cite`, `subject`, `retrieve`, `route`, `api`, `eval` | Pin theo commit, PR lên upstream |
| HTTP | `net/http`, SSE, OpenAPI 3.1 | Không cần framework |
| CSDL | PostgreSQL 17 + pgvector (HNSW), `pg_trgm`, `unaccent` | `pgx` v5, `goose` |
| Embedding | Cloud API cấu hình được; chọn bằng benchmark ở Phase 1 | Nhiều model cùng tồn tại |
| Rerank | Tùy chọn, mặc định tắt cho tới khi có số liệu | |
| LLM | Anthropic Messages API, endpoint tương thích OpenAI | Thêm wire Anthropic và tool calling vào `route` |
| Tích hợp agent | MCP server (Go SDK chính thức), `/v1/chat/completions` | |
| Frontend | React 19, TypeScript, Vite, TanStack Router + Query, Tailwind CSS, shadcn/ui, Zustand, react-markdown, react-i18next | Nhúng bằng `go:embed` |
| Bảo mật | AES-GCM cho API key, master key từ env; session cookie | Vai trò ở Phase 4 |
| Kiểm thử | `go test` + testcontainers-go, Vitest, Playwright | CI trên Linux, Windows, macOS |
| Quan sát | `log/slog` JSON, bảng `usage`; OpenTelemetry tùy chọn | |
| Triển khai | Dockerfile nhiều stage, docker-compose (app + Postgres), goreleaser | |
| CI/CD | GitHub Actions | |

## Lộ trình

| Phase | Tuần | Mốc | Web UI |
| --- | --- | --- | --- |
| P0 Nền móng dữ liệu | 1–3 | Corpus v1, khung UI | Khung, layout, theme |
| P1 Lõi truy xuất | 4–6 | Có baseline recall@10 | Retrieval Testing |
| P2 RAG MVP + Chat | 7–10 | MVP dùng nội bộ | Chat, trích dẫn, cài đặt LLM |
| P3 Agent + công cụ | 11–14 | Đạt ngưỡng gold set | Tra HS, các bước agent |
| P4 Quản trị, hoàn thiện | 15–17 | Beta cho chuyên gia | Quản trị, feedback, dashboard |
| P5 Phát hành v1.0 | 18–19 | Tag v1.0.0 công khai | Trang giới thiệu, docs |

Gold set của chuyên gia cần về trước tuần 11. Thời lượng giả định một người phát triển toàn thời gian cùng Claude.

## Phase 0 — Khởi tạo repo và nền móng dữ liệu (tuần 1–3)

Mục tiêu: corpus hải quan sinh ra bằng lệnh Go, chạy lại luôn ra cùng kết quả; khung Web UI được binary phục vụ.

Backend và dữ liệu:

- [x] Tạo repo, Go module, `LICENSE` (MIT), `NOTICE` ghi nguồn (vbpl.vn qua th1nhng0 CC-BY-4.0, UTS_VLC MIT)
- [x] Import luatdo, pin theo commit `42cd481`
- [x] `CONTRIBUTING.md`; CI lint và test trên Linux, Windows, macOS
- [x] `customsvn prep` port từ `prototypes/`: `tier`, `rescue`, `vbhn`, `hs`, `chunks` — đã khớp baseline của prototype, sau đó sửa 6 lỗi; số liệu mới và các lỗi ở [corpus.md](corpus.md)
- [x] Tách bảng HTML dùng chung: `colspan`/`rowspan`, Markdown, lặp tiêu đề ở mỗi đoạn — `internal/htmltable`, bước `prep tables`: 3.433 bảng thành 5.770 chunk bảng
- [x] ~~PR lên luatdo~~ — bỏ (quyết định 2026-10-27): không PR lên luatdo; lọc hiệu lực, chuẩn hóa số hiệu và ID VBHN đã làm trong `internal/corpus`
- [x] Danh mục khoảng 100 văn bản hải quan bắt buộc kèm báo cáo độ phủ — 95 văn bản (bản nháp chờ chuyên gia duyệt), [coverage.md](coverage.md): 84 có, 8 hết hiệu lực, 3 thiếu vì nguồn chỉ có metadata
- [x] Schema `tariffs`; ~~tìm nguồn biểu thuế XNK chính thức~~ — schema có trong migration; bỏ việc tìm nguồn (quyết định 2026-10-27), câu hỏi thuế suất của mã hàng bị từ chối có hướng dẫn
- [x] Kiểm tra config `legacy_content` để tìm text cho các VBHN rỗng — có text cho 219/314 VBHN rỗng (44 → 144 VBHN); bật mặc định từ 2026-10-27 (corpus version 2), `-legacy=false` để tắt
- [x] Danh sách văn bản 1995–2009 nghi ngờ về hiệu lực để chuyên gia rà soát — `report/old_in_force.csv`, 1.081 văn bản (404 tầng 1)

Web UI:

- [x] Khởi tạo `web/` (Vite, React 19, TypeScript, Tailwind, shadcn/ui), lint và format
- [x] Layout khung: sidebar, vùng nội dung, panel phải; sáng/tối; i18n tiếng Việt
- [x] Design token: màu, cỡ chữ, khoảng cách, nhãn hiệu lực
- [x] `customsvn serve` phục vụ bản build qua `go:embed` và `/healthz`

Hoàn thành khi: chạy pipeline hai lần từ dữ liệu thô ra cùng hash; mọi văn bản trong danh mục bắt buộc hoặc đã có, hoặc ghi rõ lý do thiếu.

## Phase 1 — Lõi truy xuất và Retrieval Testing (tuần 4–6)

Mục tiêu: đo được chất lượng tìm kiếm trước khi có chatbot; chọn model embedding bằng số liệu.

Backend:

- [x] Migration toàn bộ schema; `customsvn ingest` theo hash, sinh `corpus_version`
- [x] Interface embedding và 2–3 provider cloud để benchmark; `customsvn embed` chỉ tính phần thay đổi, có retry và giới hạn tốc độ — OpenAI-compatible, Voyage, Gemini; benchmark chờ API key
- [x] Hybrid search: BM25 luatdo + pgvector + RRF, rerank tùy chọn; lọc tầng, hiệu lực, ngày, loại văn bản; ưu tiên VBHN
- [x] Package `refs`: tham chiếu điều khoản, số hiệu, mã HS
- [x] API Retrieval Testing: một truy vấn, nhập gold set, chạy hàng loạt, lưu và so sánh lần chạy
- [x] Metric: recall@k, MRR, nDCG@10, tỷ lệ chunk hết hiệu lực trong top-10, độ trễ p50/p95
- [x] Gold set tạm khoảng 40 câu trong lúc chờ chuyên gia — `goldsets/provisional.csv`, 45 câu

Web UI:

- [x] Retrieval Testing một truy vấn: ô nhập, bộ lọc, kết quả kèm điểm từng thành phần, xem text chunk
- [x] Chế độ A/B: hai cấu hình cạnh nhau
- [x] Chế độ hàng loạt: tải gold set (CSV/JSON), xem trước dòng lỗi, thanh tiến độ, bảng metric, câu trượt, lịch sử và so sánh
- [x] Trang cài đặt truy xuất

Hoàn thành khi: có baseline recall@10 trên gold set tạm; top-10 mặc định không có chunk hết hiệu lực; nhập được gold set chuyên gia ngay khi nhận.

Kết quả (2026-10-27, chỉ BM25 vì chưa có API key embedding): recall@10 0,805, hit@10 0,829, MRR 0,576, chunk hết hiệu lực trong top-10 bằng 0. Chi tiết ở [retrieval.md](retrieval.md). Còn lại: benchmark embedding khi có key.

## Phase 2 — Chatbot RAG MVP và Web UI chat (tuần 7–10)

Mục tiêu: `docker compose up`, mở trình duyệt, hỏi về thủ tục hải quan và nhận câu trả lời có trích dẫn đã kiểm chứng.

Backend:

- [x] Lớp LLM: bọc `route` của luatdo, thêm wire Anthropic và tool calling chung; streaming; giữ failover
- [x] API key: env hoặc nhập trong UI, lưu mã hóa, che khi hiển thị, test kết nối
- [x] Composer và prompt hệ thống tiếng Việt; trả nhận định kèm trích dẫn
- [x] Verifier: câu trích nguyên văn, điều khoản còn hiệu lực; không đủ căn cứ thì từ chối
- [x] Hội thoại nhiều lượt: viết lại câu hỏi theo ngữ cảnh; lưu `conversations`, `messages`, `citations`
- [x] API: `POST /api/chat` (SSE: token, citation, done), hội thoại, văn bản, điều khoản, feedback
- [x] `docker-compose.yml` (app + Postgres); một lệnh nạp dữ liệu lần đầu

Web UI:

- [x] Chat: streaming, markdown, trích dẫn [n], dừng/tạo lại, gợi ý câu hỏi
- [x] Panel trích dẫn: text, đường dẫn, nhãn hiệu lực, "đã hợp nhất trong", chú thích sửa đổi, link vbpl.vn
- [x] Trình xem văn bản: cây cấu trúc, cuộn tới và highlight đoạn trích
- [x] Sidebar lịch sử: tạo, đổi tên, xóa, tìm kiếm
- [x] Cài đặt LLM: provider, model theo vai trò, API key, test kết nối
- [x] Feedback 👍/👎 kèm lý do; dòng miễn trừ "Thông tin tra cứu, không phải tư vấn pháp luật" kèm ngày dữ liệu
- [x] Trạng thái lỗi và rỗng: mất kết nối LLM, hết quota, bị từ chối

Hoàn thành khi: mọi câu trả lời có ít nhất một trích dẫn hợp lệ hoặc là lời từ chối; không trích văn bản hết hiệu lực; p95 thời gian ra token đầu tiên dưới 3 giây.

Kết quả (2026-10-27): đủ chức năng, kiểm thử đầu-cuối với model giả và test tự động; câu trả lời stream theo từng nhận định đã kiểm chứng thay vì từng token (xem [chat.md](chat.md)). Còn đo với model thật khi có API key: tỷ lệ câu có trích dẫn hợp lệ và p95 thời gian ra nhận định đầu tiên.

## Phase 3 — Agent và công cụ hải quan (tuần 11–14)

Mục tiêu: agent tự chọn công cụ, trả lời câu hỏi nhiều bước, gọi được từ ngoài qua MCP.

Backend:

- [x] Intent router (model rẻ); vòng lặp tool tối đa 6 bước; trace
- [x] Công cụ `resolve_reference`, `get_document_status`, `get_amendments`, `lookup_hs`, `graph_query` — cùng `search_legal`, `get_provision`; trạng thái và quan hệ đọc từ bảng `registry` (128 nghìn văn bản) và đồ thị quan hệ chính thức của vbpl.vn, bổ sung cạnh suy từ tiêu đề
- [x] Trả lời theo mốc ngày; hỏi lại khi thiếu loại hình, xuất xứ, đối tượng
- [x] Nạp `hs_codes`; tìm theo mã và mô tả (trigram + vector) — vector chạy khi đã `embed -target hs` (chờ API key)
- [x] MCP server (stdio và HTTP); `/v1/chat/completions`
- [ ] Chạy gold set chuyên gia qua truy xuất và trả lời; chốt ngưỡng phát hành theo baseline — chờ gold set chuyên gia và API key

Web UI:

- [x] Các bước agent dạng thu gọn
- [x] Tra cứu HS: tìm, cây phân cấp, chú giải chương, song ngữ, link văn bản nguồn — chưa có chú giải chương: dữ liệu nguồn không tách được phần chú giải
- [x] Thẻ trong chat: trạng thái văn bản, chuỗi sửa đổi dạng dòng thời gian, kết quả HS
- [x] Chọn mốc ngày cho hội thoại
- [x] Trang hướng dẫn kết nối MCP và API

Hoàn thành khi: đạt ngưỡng gold set (đề xuất recall@10 ≥ 0,85, trích dẫn đúng ≥ 0,95, chốt lại sau baseline).

Kết quả (2026-10-28): đủ chức năng, kiểm thử đầu-cuối với model giả và test tự động ([agent.md](agent.md)). Chưa đo ngưỡng: cần gold set chuyên gia và API key.

## Phase 4 — Web UI quản trị và hoàn thiện sản phẩm (tuần 15–17)

Mục tiêu: bản beta cho nhóm chuyên gia, đủ công cụ quản trị dữ liệu và cải thiện từ feedback.

Backend:

- [x] Đăng nhập session cookie; mặc định một admin; tùy chọn nhiều người dùng, vai trò admin/user
- [x] Giới hạn tốc độ; ghi token và chi phí vào `usage`
- [x] Job nền cho ingest/embed: hàng đợi, tiến độ, log — thêm cả prep
- [x] Chống prompt injection: nội dung văn bản là dữ liệu, không phải chỉ dẫn; bộ test tấn công mẫu — rào câu hỏi và nguồn, loại nhận định có link/ảnh/HTML; `internal/answer/injection_test.go`, `goldsets/injection.csv`
- [x] Xuất/nhập cấu hình (không kèm secret)
- [x] Đánh giá câu trả lời end-to-end: trung thành nguồn, trích dẫn đúng, từ chối đúng lúc — chấm bằng code, không dùng model làm giám khảo

Web UI:

- [x] Quản trị dữ liệu: phiên bản corpus, thống kê, độ phủ, chạy ingest/embed, log
- [x] Feedback và review: danh sách 👎, xem lại hội thoại kèm trace, gắn nhãn, chuyển thành gold item
- [x] `/lab/answers`: đánh giá câu trả lời, so sánh cấu hình model
- [x] Dashboard: số câu hỏi, tỷ lệ từ chối, độ trễ, chi phí theo ngày
- [x] Đăng nhập, quản lý người dùng
- [x] Hoàn thiện UX: mobile, phím tắt, accessibility, trạng thái loading
- [x] Playwright cho các luồng chính — 13 test, chạy với máy chủ thử và model giả (`web/e2e`)

Hoàn thành khi: chuyên gia dùng một tuần, mọi 👎 được xử lý qua màn hình review; Playwright pass.

Kết quả (2026-10-28): đủ chức năng ([admin.md](admin.md)); Playwright pass 13/13 với model giả. Còn lại: tuần dùng thử của chuyên gia.

## Phase 5 — Phát hành open source v1.0 (tuần 18–19)

Mục tiêu: người lạ cài được từ đầu, chỉ đọc tài liệu, trong dưới 15 phút.

- [ ] README có ảnh chụp, hướng dẫn cài Windows/Linux/macOS, cấu hình LLM, kiến trúc, quy trình nạp dữ liệu
- [ ] Release: binary 3 hệ điều hành (goreleaser), Docker image, `docker-compose.yml` mẫu, dữ liệu demo nhỏ
- [ ] Công bố corpus lên Hugging Face kèm dataset card, CC-BY-4.0 theo nguồn, ghi nhận đầy đủ
- [ ] Bảo mật: rà soát lưu key, kiểm tra dependency, `SECURITY.md`
- [ ] Ghi nguồn và miễn trừ trong UI (trang Giới thiệu)
- [ ] `CODE_OF_CONDUCT.md`, mẫu issue/PR, roadmap công khai, nhãn "good first issue"
- [ ] Thẩm định cuối: toàn bộ gold set, cài mới trên máy sạch cả 3 hệ điều hành

Hoàn thành khi: cài mới theo tài liệu dưới 15 phút; gold set đạt ngưỡng đã chốt ở Phase 3.

## Phase 6+ — Dữ liệu sống và mở rộng (sau v1.0)

| Hạng mục | Nội dung | Điều kiện |
| --- | --- | --- |
| Dữ liệu sống | Crawler sitemap vbpl.vn theo `lastmod`, tuân thủ robots.txt, giới hạn tốc độ; Công báo, công văn customs.gov.vn, file đính kèm VBHN; ingest tăng dần; cảnh báo đổi trạng thái; lịch cập nhật trong UI | Xác nhận điều khoản sử dụng từng nguồn |
| LLM local | Ollama/vLLM, embedding local, cấu hình trong UI, so sánh với cloud trên gold set | GPU hoặc chấp nhận độ trễ |
| Tính thuế | Trị giá × thuế suất (MFN, ưu đãi, FTA) + VAT, TTĐB, BVMT; gợi ý mẫu C/O | Có dữ liệu `tariffs` |
| Lớp norm | Pass norm của luatdo: nghĩa vụ, hạn chót, chế tài; phát hiện mâu thuẫn | Ngân sách API cho campaign `tax-2025` |

## Tiêu chí chất lượng và rủi ro

| Chỉ số | Cách đo | Ngưỡng đề xuất |
| --- | --- | --- |
| Trích dẫn văn bản hết hiệu lực | Tỷ lệ trích dẫn tới văn bản không còn hiệu lực tại mốc ngày | 0 |
| Recall@10 | Điều khoản kỳ vọng nằm trong top 10 | ≥ 0,85 |
| Trích dẫn đúng | Câu trích khớp nguyên văn, đúng điều khoản | ≥ 0,95 |
| Từ chối đúng | Câu ngoài phạm vi hoặc không đủ căn cứ bị từ chối | ≥ 0,90 |
| Độ trễ | p95 thời gian ra token đầu tiên | < 3 giây |

| Rủi ro | Ảnh hưởng | Giảm thiểu |
| --- | --- | --- |
| Trạng thái hiệu lực trong nguồn sai hoặc chậm | Trả lời bằng quy định cũ | Chuyên gia rà soát; hiển thị ngày dữ liệu; crawl ở Phase 6 |
| Thiếu biểu thuế và công văn | Không trả lời được thuế suất | Nói rõ phạm vi; từ chối có hướng dẫn |
| LLM bịa nội dung | Sai lệch pháp lý | Verifier; từ chối khi thiếu căn cứ |
| Embedding kém tiếng Việt | Bỏ sót điều khoản | Hybrid với BM25; benchmark Phase 1 |
| Chi phí API vượt dự kiến | Vượt ngân sách | Model rẻ cho router; giới hạn token; dashboard chi phí |
| Prompt injection trong văn bản | Agent bị điều khiển sai | Tách vai dữ liệu/chỉ dẫn; test tấn công mẫu |
| luatdo đổi API | Build lỗi | Pin commit; PR lên upstream |
| Người dùng coi là tư vấn pháp luật | Trách nhiệm pháp lý | Miễn trừ trong UI; luôn kèm trích dẫn và ngày dữ liệu |

## Việc chủ dự án cần chuẩn bị

Gold set 150–300 câu. `expected_provisions` viết tham chiếu dạng chữ, hệ thống tự chuyển sang ID; nhiều điều khoản ngăn cách bằng `;`.

```csv
id,question,expected_provisions,expected_answer,as_of_date,category,should_refuse,notes
q001,"Người khai hải quan có được yêu cầu xác định trước mã số hàng hóa không?","điểm b khoản 1 Điều 18 Luật Hải quan 54/2014/QH13","Có, khi đã cung cấp đầy đủ, chính xác thông tin cho cơ quan hải quan",2026-10-27,quyen-nghia-vu,false,
q002,"Thuế suất nhập khẩu ưu đãi của mã 8703.23.61 là bao nhiêu?","","",2026-10-27,thue-suat,true,"chưa có dữ liệu biểu thuế, phải từ chối"
```

Phân bổ theo nhóm: thủ tục, thuế XNK, trị giá, xuất xứ và FTA, mã HS, kiểm tra chuyên ngành, xử phạt, trạng thái văn bản, câu hỏi theo mốc ngày; thêm khoảng 10% câu phải bị từ chối.

Quyết định còn mở:

- [x] Tên dự án và repo: `CustomsVN`
- [ ] Provider LLM mặc định và ngân sách API hàng tháng
- [ ] Provider embedding (chốt sau benchmark Phase 1)
- [ ] Demo công khai hay chỉ hỗ trợ tự cài
- [x] ~~Nguồn biểu thuế XNK chính thức~~ — bỏ
- [ ] Người rà soát văn bản nghi ngờ hiệu lực, hạn gửi gold set
- [ ] Tạo repo trên GitHub: public hay private đến cuối Phase 2
