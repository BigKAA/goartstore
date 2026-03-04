// loader.go — загрузка каталогов переводов из embed.FS.
package i18n

import (
	"fmt"
	"log/slog"
)

// LoadFromEmbedFS загружает все каталоги переводов из встроенной файловой системы.
// Ожидаемые файлы: locales/en.json, locales/ru.json.
func LoadFromEmbedFS(bundle *Bundle, logger *slog.Logger) error {
	langs := []string{"en", "ru"}

	for _, lang := range langs {
		path := fmt.Sprintf("locales/%s.json", lang)
		data, err := LocaleFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("i18n: не удалось прочитать %s: %w", path, err)
		}

		if err := bundle.LoadMessages(lang, data); err != nil {
			return err
		}
	}

	logger.Info("i18n каталоги загружены", slog.Int("languages", len(langs)))
	return nil
}
