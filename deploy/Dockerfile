# ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS build
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO disabled → fully static binary (modernc.org/sqlite is pure Go, no CGO).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/daycore ./cmd/daycore

# ── runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app \
 && mkdir -p /app/data && chown app /app/data
WORKDIR /app
COPY --from=build /out/daycore /app/daycore
COPY config/ /app/config/
USER app
EXPOSE 8080
ENV APP_ENV=production \
    DB_TYPE=sqlite \
    DB_DSN="file:/app/data/daycore.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
VOLUME ["/app/data"]
ENTRYPOINT ["/app/daycore"]
