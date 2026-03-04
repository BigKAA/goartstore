package gateway

import (
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// Download скачивает файл через Query Module.
// GET /query/api/v1/files/{fileID}/download
//
// Возвращает DownloadResponse с потоком данных — вызывающий обязан закрыть Body.
// При HTTP 410 возвращает ErrFileArchived.
// При HTTP 404 возвращает ErrNotFound.
func (c *Client) Download(fileID string) (*DownloadResponse, error) {
	url := c.baseURL + "/query/api/v1/files/" + fileID + "/download"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}

	resp, err := c.doRequest(req, false)
	if err != nil {
		return nil, err
	}

	// Обработка ошибок — при не-200 возвращаем типизированную ошибку.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, c.handleErrorResponse(resp)
	}

	// Парсим метаданные из заголовков ответа.
	contentType := resp.Header.Get("Content-Type")
	contentLength, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	etag := resp.Header.Get("ETag")
	filename := extractFilename(resp.Header.Get("Content-Disposition"))

	return &DownloadResponse{
		Body:          resp.Body,
		ContentType:   contentType,
		ContentLength: contentLength,
		Filename:      filename,
		ETag:          etag,
	}, nil
}

// extractFilename извлекает имя файла из заголовка Content-Disposition.
// Формат: attachment; filename="example.txt"
func extractFilename(disposition string) string {
	if disposition == "" {
		return ""
	}

	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		// Fallback: попробуем извлечь вручную.
		if idx := strings.Index(disposition, "filename="); idx >= 0 {
			name := disposition[idx+len("filename="):]
			name = strings.Trim(name, `"`)
			return name
		}
		return ""
	}

	return params["filename"]
}
