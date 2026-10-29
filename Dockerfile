# customsvn with its Web UI embedded, the pinned luatdo binary beside it for
# the first data load (customsvn bootstrap) and the weekly update, and the
# dataset's vbpl.vn crawler.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/customsvn ./cmd/customsvn
# luatdo at exactly the commit go.mod pins.
RUN CGO_ENABLED=0 GOBIN=/out go install -trimpath "github.com/tamnd/luatdo/cmd/luatdo@$(go list -m -f '{{.Version}}' github.com/tamnd/luatdo)"

# The weekly crawl runs the crawler published with the th1nhng0 dataset
# (Scrapy, polars, pyarrow), at the revision the corpus is built on and with
# the versions its lock file pins; it is downloaded, not copied into this repo.
FROM python:3.12-slim
ARG DATASET_REVISION=8977887f17be2defae4c5171d55562e1cde7d695
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata curl \
 && rm -rf /var/lib/apt/lists/* && useradd -m -u 10001 customsvn
RUN set -e; \
    base="https://huggingface.co/datasets/th1nhng0/vietnamese-legal-documents/resolve/${DATASET_REVISION}/crawler"; \
    for f in scrapy.cfg pyproject.toml uv.lock build_full_dataset.py upsert_dataset.py \
             vbpl/__init__.py vbpl/api.py vbpl/middlewares.py vbpl/settings.py \
             vbpl/spiders/__init__.py vbpl/spiders/vbpl.py; do \
      curl -fsSL --create-dirs -o "/opt/vbpl-crawler/$f" "$base/$f"; \
    done; \
    pip install --no-cache-dir uv; \
    cd /opt/vbpl-crawler; \
    uv export --frozen --no-dev --no-hashes --no-emit-project -o requirements.txt; \
    pip install --no-cache-dir -r requirements.txt; \
    pip uninstall -y uv; \
    chown -R customsvn /opt/vbpl-crawler
COPY --from=build /out/customsvn /out/luatdo /usr/local/bin/
ENV LUATDO_DATA=/data TZ=Asia/Ho_Chi_Minh CUSTOMSVN_CRAWLER=/opt/vbpl-crawler CUSTOMSVN_PYTHON=python3
RUN mkdir -p /data && chown customsvn /data
USER customsvn
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["customsvn"]
CMD ["serve", "-listen", "0.0.0.0:8080"]
