# Multi-stage сборка Storage Element
# Этап 1: Сборка бинарника
FROM golang:1.24-alpine AS builder

ARG VERSION=dev

RUN apk add --no-cache git

WORKDIR /build

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Сборка бинарника со статической линковкой
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X github.com/arturkryukov/artsore/storage-element/internal/config.Version=${VERSION}" \
    -o /storage-element \
    ./cmd/storage-element/

# Этап 2: Минимальный образ для запуска
FROM alpine:3.19

# Создаём непривилегированного пользователя
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Устанавливаем ca-certificates для TLS-соединений
RUN apk add --no-cache ca-certificates

# Копируем бинарник из builder
COPY --from=builder /storage-element /usr/local/bin/storage-element

# Создаём директории для данных и WAL
RUN mkdir -p /data /wal /certs && \
    chown -R appuser:appgroup /data /wal /certs

# Переключаемся на непривилегированного пользователя
USER appuser

# Порт по умолчанию
EXPOSE 8010

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-check-certificate -q -O /dev/null https://localhost:8010/health/live || exit 1

ENTRYPOINT ["storage-element"]
