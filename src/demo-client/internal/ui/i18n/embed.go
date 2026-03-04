package i18n

import "embed"

// LocaleFS — встроенные JSON-каталоги переводов.
// Каталоги загружаются при компиляции через go:embed.
//
//go:embed locales/*.json
var LocaleFS embed.FS
