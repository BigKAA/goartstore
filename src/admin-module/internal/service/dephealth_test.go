// dephealth_test.go — unit-тесты для функций нормализации и парсинга SE endpoints.
package service

import (
	"testing"
)

// TestNormalizeSEDepName проверяет нормализацию имён SE для dephealth.
func TestNormalizeSEDepName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "простое имя lowercase",
			input:    "storage-one",
			expected: "storage-one",
		},
		{
			name:     "верхний регистр",
			input:    "Storage-One",
			expected: "storage-one",
		},
		{
			name:     "пробелы заменяются на дефис",
			input:    "storage element one",
			expected: "storage-element-one",
		},
		{
			name:     "спецсимволы заменяются на дефис",
			input:    "se@prod#1.2",
			expected: "se-prod-1-2",
		},
		{
			name:     "множественные дефисы коллапсируются",
			input:    "se---prod---one",
			expected: "se-prod-one",
		},
		{
			name:     "trim дефисов по краям",
			input:    "---se-one---",
			expected: "se-one",
		},
		{
			name:     "начинается с цифры — префикс se-",
			input:    "1st-storage",
			expected: "se-1st-storage",
		},
		{
			name:     "только цифра",
			input:    "42",
			expected: "se-42",
		},
		{
			name:     "имя ровно 63 символа — не обрезается",
			input:    "abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-123456789",
			expected: "abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-123456789",
		},
		{
			name:     "имя длиннее 63 символов обрезается",
			input:    "abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-1234567890-extra",
			expected: "abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-123456789",
		},
		{
			name:     "пустая строка → unknown-se",
			input:    "",
			expected: "unknown-se",
		},
		{
			name:     "только спецсимволы → unknown-se",
			input:    "!!!@@@",
			expected: "unknown-se",
		},
		{
			name:     "unicode символы заменяются",
			input:    "хранилище-1",
			expected: "se-1",
		},
		{
			name:     "trailing дефис после обрезки удаляется",
			input:    "a-bcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-123456789",
			expected: "a-bcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSEDepName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeSEDepName(%q) = %q, ожидалось %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParseSEURL проверяет парсинг URL Storage Element.
func TestParseSEURL(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantHost   string
		wantPort   string
		wantTLS    bool
		wantErr    bool
	}{
		{
			name:     "HTTPS с портом",
			input:    "https://se1.example.com:8443",
			wantHost: "se1.example.com",
			wantPort: "8443",
			wantTLS:  true,
		},
		{
			name:     "HTTPS без порта — дефолт 443",
			input:    "https://se1.example.com",
			wantHost: "se1.example.com",
			wantPort: "443",
			wantTLS:  true,
		},
		{
			name:     "HTTP с портом",
			input:    "http://se1.example.com:8010",
			wantHost: "se1.example.com",
			wantPort: "8010",
			wantTLS:  false,
		},
		{
			name:     "HTTP без порта — дефолт 80",
			input:    "http://se1.example.com",
			wantHost: "se1.example.com",
			wantPort: "80",
			wantTLS:  false,
		},
		{
			name:     "HTTPS с path (path игнорируется)",
			input:    "https://se1.example.com:8443/api/v1/info",
			wantHost: "se1.example.com",
			wantPort: "8443",
			wantTLS:  true,
		},
		{
			name:     "IP-адрес с портом",
			input:    "http://192.168.1.100:8010",
			wantHost: "192.168.1.100",
			wantPort: "8010",
			wantTLS:  false,
		},
		{
			name:    "пустой URL",
			input:   "",
			wantErr: true,
		},
		{
			name:    "URL без схемы — нет host",
			input:   "se1.example.com:8010",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, tls, err := parseSEURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSEURL(%q) — ожидалась ошибка, получено: host=%q port=%q", tt.input, host, port)
				}
				return
			}
			if err != nil {
				t.Errorf("parseSEURL(%q) — неожиданная ошибка: %v", tt.input, err)
				return
			}
			if host != tt.wantHost {
				t.Errorf("parseSEURL(%q) host = %q, ожидалось %q", tt.input, host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("parseSEURL(%q) port = %q, ожидалось %q", tt.input, port, tt.wantPort)
			}
			if tls != tt.wantTLS {
				t.Errorf("parseSEURL(%q) tls = %v, ожидалось %v", tt.input, tls, tt.wantTLS)
			}
		})
	}
}
