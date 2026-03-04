package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
)

// Upload загружает файл через Ingester Module.
// POST /upload/api/v1/files/upload (multipart/form-data)
//
// Параметры:
//   - filename — имя файла для Content-Disposition
//   - fileReader — поток данных файла
//   - fileSize — размер файла в байтах (для Content-Length, 0 если неизвестен)
//   - params — дополнительные параметры загрузки (описание, теги, retention)
func (c *Client) Upload(filename string, fileReader io.Reader, _ int64, params UploadParams) (*UploadResult, error) {
	// Формируем multipart/form-data запрос.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Добавляем файл.
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания multipart file part: %w", err)
	}
	if _, err = io.Copy(part, fileReader); err != nil {
		return nil, fmt.Errorf("ошибка копирования данных файла: %w", err)
	}

	// Добавляем параметры как form fields.
	if params.Description != "" {
		if err = writer.WriteField("description", params.Description); err != nil {
			return nil, fmt.Errorf("ошибка записи поля description: %w", err)
		}
	}

	if len(params.Tags) > 0 {
		var tagsJSON []byte
		tagsJSON, err = json.Marshal(params.Tags)
		if err != nil {
			return nil, fmt.Errorf("ошибка сериализации тегов: %w", err)
		}
		if err = writer.WriteField("tags", string(tagsJSON)); err != nil {
			return nil, fmt.Errorf("ошибка записи поля tags: %w", err)
		}
	}

	retentionPolicy := params.RetentionPolicy
	if retentionPolicy == "" {
		retentionPolicy = "temporary"
	}
	if err = writer.WriteField("retention_policy", retentionPolicy); err != nil {
		return nil, fmt.Errorf("ошибка записи поля retention_policy: %w", err)
	}

	if params.TTLDays > 0 {
		if err = writer.WriteField("ttl_days", strconv.Itoa(params.TTLDays)); err != nil {
			return nil, fmt.Errorf("ошибка записи поля ttl_days: %w", err)
		}
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("ошибка закрытия multipart writer: %w", err)
	}

	// Создаём HTTP запрос.
	url := c.baseURL + "/upload/api/v1/files/upload"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Выполняем запрос с upload-клиентом (увеличенный таймаут).
	resp, err := c.doRequest(req, true)
	if err != nil {
		return nil, err
	}

	// Обрабатываем ответ.
	if resp.StatusCode != http.StatusCreated {
		return nil, c.handleErrorResponse(resp)
	}
	defer resp.Body.Close()

	var result UploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа upload: %w", err)
	}

	return &result, nil
}
