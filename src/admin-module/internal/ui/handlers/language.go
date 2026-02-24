// language.go — обработчик переключения языка UI.
package handlers

import (
	"net/http"
	"time"

	"github.com/bigkaa/goartstore/admin-module/internal/ui/i18n"
)

// HandleSetLanguage обрабатывает POST /admin/set-language.
// Устанавливает cookie "lang" и перенаправляет обратно.
// Параметр lang: "en" или "ru" (из query или form).
func HandleSetLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.FormValue("lang")
	if lang == "" {
		lang = r.URL.Query().Get("lang")
	}

	// Валидация: только поддерживаемые языки
	if lang != "en" && lang != "ru" {
		lang = "en"
	}

	// Устанавливаем cookie "lang" на 1 год
	http.SetCookie(w, &http.Cookie{
		Name:     i18n.LangCookieName,
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 год
		HttpOnly: false,               // JS может читать для UI-логики
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})

	// Redirect обратно на предыдущую страницу (Referer) или на /admin/
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/admin/"
	}

	http.Redirect(w, r, referer, http.StatusSeeOther)
}
