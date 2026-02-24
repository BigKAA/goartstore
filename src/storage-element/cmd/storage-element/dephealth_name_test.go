// Тесты функции parseOwnerName — извлечение имени владельца пода из hostname.
package main

import "testing"

func TestParseOwnerName(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{
			name:     "Deployment — admin-module",
			hostname: "admin-module-7d8f9b6c4f-x2k9z",
			want:     "admin-module",
		},
		{
			name:     "Deployment — storage-element с длинным именем",
			hostname: "storage-element-se-01-5fbcd8d7b9-k4m2j",
			want:     "storage-element-se-01",
		},
		{
			name:     "StatefulSet — ordinal 0",
			hostname: "my-sts-0",
			want:     "my-sts",
		},
		{
			name:     "StatefulSet — ordinal 42",
			hostname: "my-sts-42",
			want:     "my-sts",
		},
		{
			name:     "Fallback — простое имя",
			hostname: "my-app",
			want:     "my-app",
		},
		{
			name:     "Fallback — localhost",
			hostname: "localhost",
			want:     "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOwnerName(tt.hostname)
			if got != tt.want {
				t.Errorf("parseOwnerName(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
		})
	}
}
