# Đóng góp cho CustomsVN

Cảm ơn bạn quan tâm. Dự án đang ở giai đoạn đầu; lộ trình và việc đang làm nằm trong [docs/PLAN.md](docs/PLAN.md).

## Chuẩn bị

- Go ≥ 1.26.5.
- Dữ liệu nằm ngoài repo, trong một thư mục dữ liệu luatdo. Đặt biến `LUATDO_DATA` trỏ tới thư mục đó. Cách dựng dữ liệu: xem [luatdo](https://github.com/tamnd/luatdo), sau đó chạy `customsvn prep` ([docs/corpus.md](docs/corpus.md)).
- Test không cần mạng, dữ liệu hay API key.

```sh
go build ./...
go vet ./...
go test ./...
gofmt -l .        # phải không in ra gì

cd web
npm ci
npm run build && npm run lint && npm run format:check && npm test
```

Cơ sở dữ liệu cho phát triển: `docker compose -f deploy/docker-compose.yml up -d`, rồi đặt `CUSTOMSVN_DATABASE_URL` như ghi trong file đó. Cấu hình: chép `customsvn.example.yaml` thành `customsvn.yaml`. Quy trình đầy đủ ở [docs/retrieval.md](docs/retrieval.md).

## Nguyên tắc không thay đổi

- Không dùng văn bản đã hết hiệu lực làm căn cứ cho pháp luật hiện hành. Lọc theo trạng thái và ngày hết hiệu lực, không chỉ theo nhãn.
- Câu trả lời chỉ hiển thị khi mọi câu trích dẫn đã được kiểm khớp nguyên văn với chunk được dẫn.
- Cấu trúc, định danh, hiệu lực và quan hệ sửa đổi đến từ code tất định, không đến từ model.
- Mọi thay đổi ở khâu truy xuất phải chạy Retrieval Testing; PR ghi metric trước và sau.
- Không commit dữ liệu, embedding hay API key. File cấu hình không chứa bí mật.
- Nguồn dữ liệu mới phải có mục trong `NOTICE` trước khi nạp.

## Quy ước code

- Bố cục theo `docs/PLAN.md`: `cmd/customsvn`, `internal/...`, `web/`, `migrations/`.
- Theo phong cách của luatdo: comment giải thích vì sao, lỗi được trả về kèm ngữ cảnh.
- Code, định danh và commit message viết bằng tiếng Anh; chữ trên giao diện là tiếng Việt qua khóa i18n.
- luatdo được import như Go module và pin theo commit. Dự án không gửi PR lên luatdo: lỗi của luatdo được xử lý trong CustomsVN, ghi chú ngay tại chỗ xử lý.
- Thay đổi ở `customsvn prep` phải ghi số liệu trước và sau vào [docs/corpus.md](docs/corpus.md).

## Pull request

- Một PR một việc, có test cho hành vi mới hoặc lỗi được sửa.
- CI chạy `gofmt`, `go vet` và `go test` trên Linux, Windows và macOS, cùng build, lint, format và test của `web/`; PR cần qua tất cả.
- Khi xong một mục trong `docs/PLAN.md`, đánh dấu nó trong cùng PR.

Đóng góp của bạn được phát hành theo giấy phép MIT của dự án.
