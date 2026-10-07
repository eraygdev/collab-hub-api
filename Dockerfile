# ═══════════════════════════════════════════════════════
# STAGE 1: BUILDER
# ═══════════════════════════════════════════════════════
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Önce sadece go.mod + go.sum kopyala → modül cache'i daha efektif kullanılır
COPY go.mod go.sum ./
RUN go mod download

# Şimdi kaynak kodu kopyala
COPY . .

# Statik binary derle
#   CGO_ENABLED=0 → libc bağımlılığı yok (alpine'da çalışır)
#   -ldflags="-s -w" → binary boyutunu küçültür (debug bilgisi çıkar)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w" \
    -o reporeef-api .

# ═══════════════════════════════════════════════════════
# STAGE 2: RUNTIME
# ═══════════════════════════════════════════════════════
FROM alpine:3.20

# CA sertifikaları (HTTPS için), tzdata (timezone için)
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S appgroup && \
    adduser -S appuser -G appgroup

WORKDIR /app

# Sadece derlenmiş binary'i kopyala (kaynak kod image'a girmesin!)
COPY --from=builder /app/reporeef-api .

# Non-root user olarak çalıştır (güvenlik)
RUN chown -R appuser:appgroup /app
USER appuser

EXPOSE 8080

# Container healthcheck
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ping || exit 1

CMD ["./reporeef-api"]