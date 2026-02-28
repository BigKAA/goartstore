// Пакет i18n — интернационализация Admin UI.
// Предоставляет функции T(ctx, key) и Tf(ctx, key, args...) для получения
// переведённых строк из контекста HTTP-запроса.
// Поддерживаемые языки: English (en), Русский (ru).
// Язык определяется middleware: cookie "lang" → Accept-Language → default "en".
package i18n

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"golang.org/x/text/language"
)

// Поддерживаемые языки
var (
	// SupportedLanguages — список поддерживаемых тегов языков.
	SupportedLanguages = []language.Tag{
		language.English,
		language.Russian,
	}

	// matcher — языковой matcher для Accept-Language.
	matcher = language.NewMatcher(SupportedLanguages)
)

// contextKey — тип ключа для контекста (избегаем коллизий).
type contextKey string

const (
	// contextKeyLang — текущий язык в контексте запроса.
	contextKeyLang contextKey = "i18n_lang"
)

// Bundle — хранилище переводов для всех языков.
// Загружается один раз при старте приложения.
type Bundle struct {
	mu       sync.RWMutex
	catalogs map[string]map[string]string // lang → key → translation
	logger   *slog.Logger
}

// NewBundle создаёт пустой Bundle.
func NewBundle(logger *slog.Logger) *Bundle {
	return &Bundle{
		catalogs: make(map[string]map[string]string),
		logger:   logger,
	}
}

// LoadMessages загружает JSON-каталог переводов для указанного языка.
// JSON формат: {"key": "translation", ...} (плоский).
func (b *Bundle) LoadMessages(lang string, data []byte) error {
	var messages map[string]string
	if err := json.Unmarshal(data, &messages); err != nil {
		return fmt.Errorf("i18n: ошибка парсинга каталога %s: %w", lang, err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.catalogs[lang] = messages

	if b.logger != nil {
		b.logger.Info("i18n каталог загружен",
			slog.String("lang", lang),
			slog.Int("keys", len(messages)),
		)
	}
	return nil
}

// Translate возвращает перевод по ключу для указанного языка.
// Если ключ не найден — возвращает ключ как есть (для отладки).
func (b *Bundle) Translate(lang, key string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Ищем в запрошенном языке
	if catalog, ok := b.catalogs[lang]; ok {
		if msg, ok := catalog[key]; ok {
			return msg
		}
	}

	// Fallback на английский
	if lang != "en" {
		if catalog, ok := b.catalogs["en"]; ok {
			if msg, ok := catalog[key]; ok {
				return msg
			}
		}
	}

	// Ключ не найден ни в одном каталоге
	return key
}

// Translatef возвращает перевод по ключу с подстановкой аргументов (fmt.Sprintf).
// Формат-строка загружается из JSON-каталога во время выполнения,
// поэтому go vet не может проверить соответствие аргументов.
func (b *Bundle) Translatef(lang, key string, args ...any) string {
	template := b.Translate(lang, key)
	if len(args) == 0 {
		return template
	}
	return formatFunc(template, args...)
}

// --- Глобальный Bundle (singleton) ---

var (
	globalBundle *Bundle
	globalOnce   sync.Once
)

// Init инициализирует глобальный Bundle. Вызывается один раз при старте.
func Init(logger *slog.Logger) *Bundle {
	globalOnce.Do(func() {
		globalBundle = NewBundle(logger)
	})
	return globalBundle
}

// GetBundle возвращает глобальный Bundle (nil если не инициализирован).
func GetBundle() *Bundle {
	return globalBundle
}

// --- Функции для использования в templ ---

// WithLang помещает язык в контекст.
func WithLang(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, contextKeyLang, lang)
}

// LangFromContext извлекает язык из контекста. Default: "en".
func LangFromContext(ctx context.Context) string {
	if lang, ok := ctx.Value(contextKeyLang).(string); ok && lang != "" {
		return lang
	}
	return "en"
}

// T возвращает перевод по ключу, используя язык из контекста.
// Основная функция для использования в .templ файлах: { i18n.T(ctx, "key") }
func T(ctx context.Context, key string) string {
	if globalBundle == nil {
		return key
	}
	return globalBundle.Translate(LangFromContext(ctx), key)
}

// Tf возвращает перевод по ключу с аргументами (fmt.Sprintf).
// Для использования в .templ файлах: { i18n.Tf(ctx, "key", arg1, arg2) }
// Формат-строка загружается из JSON-каталога, поэтому go vet printf-проверка
// не применяется — используется обёртка formatFunc.
func Tf(ctx context.Context, key string, args ...any) string {
	if globalBundle == nil {
		if len(args) == 0 {
			return key
		}
		return formatFunc(key, args...)
	}
	return globalBundle.Translatef(LangFromContext(ctx), key, args...)
}

// formatFunc — ссылка на fmt.Sprintf через переменную для обхода go vet printf-анализатора.
// go vet проверяет прямые вызовы fmt.Sprintf и их обёртки на соответствие формат-строки
// и аргументов, но формат-строки загружаются из JSON-каталогов во время выполнения,
// поэтому статическая проверка невозможна.
//
//nolint:govet // обход go vet printf-анализатора
var formatFunc = fmt.Sprintf

// MatchLanguage определяет лучший язык из Accept-Language заголовка.
// Возвращает "en" или "ru".
func MatchLanguage(acceptLanguage string) string {
	tag, _ := language.MatchStrings(matcher, acceptLanguage)
	base, _ := tag.Base()
	lang := base.String()

	// Нормализуем к поддерживаемым значениям
	switch {
	case strings.HasPrefix(lang, "ru"):
		return "ru"
	default:
		return "en"
	}
}
