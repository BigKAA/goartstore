// Пакет static — встроенные статические ресурсы Admin UI.
// Содержит CSS (Tailwind), JS (HTMX, Alpine.js, ApexCharts) и другие ассеты.
// Файлы встраиваются в бинарник через //go:embed и раздаются через HTTP.
package static

import (
	"embed"
	"io/fs"
	"net/http"
)

// content — встроенная файловая система со всеми статическими ресурсами.
// Включает поддиректории css/ и js/.
//
//go:embed css/output.css js/*.js
var content embed.FS

// FileSystem возвращает http.FileSystem для обработки запросов к /static/*.
// Файлы доступны по путям вида /static/css/output.css, /static/js/htmx.min.js и т.д.
func FileSystem() http.FileSystem {
	return http.FS(content)
}

// FS возвращает fs.FS для прямого доступа к встроенным файлам.
func FS() fs.FS {
	return content
}
