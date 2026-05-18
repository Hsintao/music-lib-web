FROM golang:1.22-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/music-lib-web ./cmd/music-lib-web

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates gosu tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --uid 10001 --create-home --home-dir /home/music music \
  && mkdir -p /app /data/Downloads \
  && chown -R music:music /app /data

WORKDIR /app

COPY --from=builder /out/music-lib-web /app/music-lib-web
COPY web /app/web
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

EXPOSE 51873

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["--addr", "0.0.0.0:51873", "--download-dir", "/data/Downloads", "--cookie-file", "/data/.music-lib-web-cookie", "--concurrency", "3"]
