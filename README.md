# CustomsVN

Chatbot và agent AI tra cứu pháp luật hải quan Việt Nam, mã nguồn mở. Mọi câu trả lời đều trích dẫn tới Điều, Khoản, Điểm cụ thể và ghi rõ hiệu lực của văn bản; không đủ căn cứ thì từ chối trả lời.

Dự án đang ở Phase 2: hỏi đáp có trích dẫn đã kiểm chứng nguyên văn, tìm kiếm kết hợp BM25 + vector trên corpus hải quan, Retrieval Testing để đo chất lượng tìm kiếm. Lộ trình: [docs/PLAN.md](docs/PLAN.md); dữ liệu: [docs/corpus.md](docs/corpus.md); truy xuất: [docs/retrieval.md](docs/retrieval.md); hỏi đáp: [docs/chat.md](docs/chat.md).

## Chạy thử

```sh
cp .env.example .env            # đặt CUSTOMSVN_MASTER_KEY=$(openssl rand -base64 32)
docker compose up -d --build
docker compose run --rm app bootstrap   # nạp dữ liệu lần đầu, vài GB
```

Mở http://localhost:8080, vào **Cài đặt LLM** thêm nhà cung cấp và model trả lời.

Thông tin chỉ dùng để tra cứu, không phải tư vấn pháp luật.

## Giấy phép

MIT, xem [LICENSE](LICENSE).
# CustomsVN
