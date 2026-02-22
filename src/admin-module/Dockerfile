# Multi-stage сборка Admin Module
# Этап 1: Сборка бинарника
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN apk add --no-cache git

WORKDIR /build

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Сборка бинарника со статической линковкой для целевой платформы
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X github.com/arturkryukov/artsore/admin-module/internal/config.Version=${VERSION}" \
    -o /admin-module \
    ./cmd/admin-module/

# Этап 2: Минимальный образ для запуска
FROM alpine:3.19

# Создаём непривилегированного пользователя
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Устанавливаем ca-certificates для TLS-соединений (к Keycloak, SE)
RUN apk add --no-cache ca-certificates

# Копируем бинарник из builder
COPY --from=builder /admin-module /usr/local/bin/admin-module

# Переключаемся на непривилегированного пользователя
USER appuser

# Порт по умолчанию (HTTP, без TLS)
EXPOSE 8000

# Health check (HTTP, без TLS — TLS на API Gateway)
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:8000/health/live || exit 1

ENTRYPOINT ["admin-module"]
