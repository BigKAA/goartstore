package handlers

import (
	"net/http"
)

// SetLanguage — POST /set-language — переключение языка UI.
// Устанавливает cookie "lang" и перенаправляет на referer.
func SetLanguage(w http.ResponseWriter, r *http.Request) {
	lang := r.FormValue("lang")
	if lang != "en" && lang != "ru" {
		lang = "ru"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lang",
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 год
		HttpOnly: false,              // доступен из JS при необходимости
		SameSite: http.SameSiteLaxMode,
	})

	// Перенаправляем на предыдущую страницу
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
