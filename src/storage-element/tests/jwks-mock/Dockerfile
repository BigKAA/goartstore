# Multi-stage сборка JWKS Mock Server
# Этап 1: Сборка бинарника
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Сборка бинарника со статической линковкой
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w" \
    -o /jwks-mock .

# Этап 2: Минимальный образ для запуска
FROM alpine:3.19

# Создаём непривилегированного пользователя
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Устанавливаем ca-certificates для TLS
RUN apk add --no-cache ca-certificates

# Копируем бинарник из builder
COPY --from=builder /jwks-mock /usr/local/bin/jwks-mock

# Создаём директорию для TLS сертификатов
RUN mkdir -p /certs && chown -R appuser:appgroup /certs

# Переключаемся на непривилегированного пользователя
USER appuser

# Порт по умолчанию
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:8080/health || exit 1

ENTRYPOINT ["jwks-mock"]
