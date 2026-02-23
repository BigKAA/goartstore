# Multi-stage сборка Admin Module
# Этапы: templ generate → Tailwind CSS compile → Go build → runtime

# =============================================================================
# Этап 1: Генерация Go-кода из .templ шаблонов
# =============================================================================
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS templ

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /build
COPY . .

# Генерируем _templ.go файлы из .templ шаблонов
RUN templ generate

# =============================================================================
# Этап 2: Компиляция Tailwind CSS
# =============================================================================
FROM --platform=linux/amd64 alpine:3.19 AS tailwind

# Скачиваем Tailwind CSS Standalone CLI
ARG TAILWIND_VERSION=v3.4.17
RUN apk add --no-cache curl && \
    curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-x64 && \
    chmod +x tailwindcss-linux-x64 && \
    mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss

WORKDIR /build

# Копируем конфиг Tailwind и CSS-файлы
COPY tailwind.config.js ./
COPY internal/ui/static/css/input.css ./internal/ui/static/css/input.css

# Копируем .templ файлы (content paths для Tailwind)
COPY internal/ui/ ./internal/ui/

# Компилируем минифицированный CSS
RUN tailwindcss -i ./internal/ui/static/css/input.css \
    -o ./internal/ui/static/css/output.css --minify

# =============================================================================
# Этап 3: Сборка Go-бинарника
# =============================================================================
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

# Копируем сгенерированные _templ.go файлы из этапа templ
COPY --from=templ /build/internal/ui/ ./internal/ui/

# Копируем скомпилированный output.css из этапа tailwind
COPY --from=tailwind /build/internal/ui/static/css/output.css ./internal/ui/static/css/output.css

# Сборка бинарника со статической линковкой для целевой платформы
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X github.com/arturkryukov/artstore/admin-module/internal/config.Version=${VERSION}" \
    -o /admin-module \
    ./cmd/admin-module/

# =============================================================================
# Этап 4: Минимальный образ для запуска
# =============================================================================
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
