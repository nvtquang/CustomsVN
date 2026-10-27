# customsvn with its Web UI embedded, and the pinned luatdo binary beside it
# for the first data load (customsvn bootstrap).

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

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 customsvn
COPY --from=build /out/customsvn /out/luatdo /usr/local/bin/
ENV LUATDO_DATA=/data TZ=Asia/Ho_Chi_Minh
RUN mkdir -p /data && chown customsvn /data
USER customsvn
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["customsvn"]
CMD ["serve", "-listen", "0.0.0.0:8080"]
