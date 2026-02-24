// middleware.go — HTTP middleware для определения языка пользователя.
// Приоритет: cookie "lang" → заголовок Accept-Language → default "en".
package i18n

import (
	"net/http"
)

// LangCookieName — имя cookie для хранения выбранного языка.
const LangCookieName = "lang"

// Middleware создаёт HTTP middleware для определения языка и помещения его в контекст.
// Приоритет: cookie "lang" → Accept-Language → default "en".
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang := detectLanguage(r)
			ctx := WithLang(r.Context(), lang)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// detectLanguage определяет язык из запроса.
// Приоритет: cookie "lang" → Accept-Language → default "en".
func detectLanguage(r *http.Request) string {
	// 1. Cookie "lang" (пользователь явно выбрал язык)
	if cookie, err := r.Cookie(LangCookieName); err == nil && cookie.Value != "" {
		lang := cookie.Value
		if lang == "en" || lang == "ru" {
			return lang
		}
	}

	// 2. Accept-Language заголовок
	if accept := r.Header.Get("Accept-Language"); accept != "" {
		return MatchLanguage(accept)
	}

	// 3. Default
	return "en"
}
