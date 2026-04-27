FROM golang:1.26 AS build

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.buildVersion=${VERSION}" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.buildVersion=${VERSION}" -o /out/api-migrate ./cmd/migrate

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN groupadd --system app && useradd --system --gid app --home-dir /app --no-create-home app \
  && mkdir -p /app/etc /app/logs /app/data \
  && chown -R app:app /app
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/api-migrate /usr/local/bin/api-migrate
COPY etc/config.sample.yaml /app/etc/config.sample.yaml

EXPOSE 8890
USER app

HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8890/api/live || exit 1

ENTRYPOINT ["/usr/local/bin/api"]
CMD ["-f", "/app/etc/config.yaml"]
