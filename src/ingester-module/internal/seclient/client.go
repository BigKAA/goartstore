// Пакет seclient — HTTP-клиент для загрузки файлов в Storage Elements.
// Поддерживает TLS с кастомным CA (IM_SE_CA_CERT_PATH), multipart upload
// через io.Pipe + streaming, sentinel errors для retry при 507.
package seclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// Sentinel errors для обработки в upload/delete pipeline.
var (
	// ErrStorageFull — SE вернул 507 (нет свободного места).
	// Upload pipeline использует для retry с другим SE.
	ErrStorageFull = fmt.Errorf("storage element: нет свободного места (507)")

	// ErrFileTooLarge — SE вернул 413 (файл превышает лимит SE).
	ErrFileTooLarge = fmt.Errorf("storage element: файл превышает допустимый размер SE (413)")

	// ErrFileNotFound — SE вернул 404 (файл не найден).
	ErrFileNotFound = fmt.Errorf("storage element: файл не найден (404)")

	// ErrModeNotAllowed — SE вернул 409 (SE не в режиме edit для удаления).
	ErrModeNotAllowed = fmt.Errorf("storage element: операция запрещена для текущего режима SE (409)")

	// ErrFileUploadInProgress — SE вернул 409 (файл в процессе загрузки).
	ErrFileUploadInProgress = fmt.Errorf("storage element: файл в процессе загрузки (409)")
)

// TokenProvider — функция, возвращающая SA-токен для авторизации запросов к SE.
// Обычно это adminclient.Client.GetToken.
type TokenProvider func(ctx context.Context) (string, error)

// UploadResult — результат успешной загрузки файла в SE.
// Содержит поля из ответа SE, без IM-level полей (retention_policy, ttl_days и т.д.).
type UploadResult struct {
	// FileID — UUID файла, присвоённый Storage Element
	FileID string `json:"file_id"`
	// OriginalFilename — оригинальное имя файла
	OriginalFilename string `json:"original_filename"`
	// ContentType — MIME-тип файла
	ContentType string `json:"content_type"`
	// Size — размер файла в байтах
	Size int64 `json:"size"`
	// Checksum — SHA-256 хэш содержимого файла
	Checksum string `json:"checksum"`
	// UploadedBy — sub из SA JWT (не конечный пользователь)
	UploadedBy string `json:"uploaded_by"`
	// UploadedAt — дата и время загрузки
	UploadedAt time.Time `json:"uploaded_at"`
	// Description — описание файла (если передано)
	Description *string `json:"description"`
	// Tags — теги файла (если переданы)
	Tags []string `json:"tags"`
	// Status — статус файла ("active")
	Status string `json:"status"`
}

// Client — HTTP-клиент для загрузки файлов в Storage Elements.
type Client struct {
	httpClient    *http.Client
	tokenProvider TokenProvider
	logger        *slog.Logger
}

// New создаёт SE-клиент для upload.
// caCertPath — путь к CA-сертификату для TLS (пустая строка — стандартный пул).
// timeout — таймаут HTTP-запросов для загрузки (из конфигурации IM_SE_UPLOAD_TIMEOUT).
// tokenProvider — функция для получения SA-токена (adminclient.Client.GetToken).
func New(caCertPath string, timeout time.Duration, tokenProvider TokenProvider, logger *slog.Logger) (*Client, error) {
	transport := &http.Transport{
		// Настройка пула idle-соединений для эффективного переиспользования
		MaxIdleConnsPerHost: 10,
	}

	if caCertPath != "" {
		tlsConfig, err := buildTLSConfig(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA-сертификата SE: %w", err)
		}
		transport.TLSClientConfig = tlsConfig
		logger.Info("CA-сертификат SE добавлен в пул доверия",
			slog.String("ca_cert", caCertPath),
		)
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	return &Client{
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
		logger:        logger.With(slog.String("component", "se_client")),
	}, nil
}

// Upload загружает файл в Storage Element через multipart/form-data.
// Использует io.Pipe + горутину для streaming без буферизации в памяти.
//
// seURL — базовый URL Storage Element (например, https://se-01:8010).
// file — io.ReadSeeker для возможности Seek(0,0) при retry.
// filename — оригинальное имя файла для Content-Disposition.
// description — описание файла (пустая строка = не передавать).
// tags — теги файла (nil = не передавать).
//
// Sentinel errors:
//   - ErrStorageFull (507) — для retry с другим SE
//   - ErrFileTooLarge (413) — SE имеет своё ограничение размера
func (c *Client) Upload(
	ctx context.Context,
	seURL string,
	file io.ReadSeeker,
	filename string,
	_ string, // contentType — SE определяет автоматически
	description string,
	tags []string,
) (*UploadResult, error) {
	// Формируем multipart body через pipe
	pr, mpWriter := c.buildMultipartPipe(file, filename, description, tags)

	// Формируем HTTP-запрос
	resp, err := c.doUploadRequest(ctx, seURL, pr, mpWriter.FormDataContentType())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Обработка ответа SE
	return c.handleUploadResponse(resp, seURL)
}

// Delete удаляет файл из Storage Element.
// seURL — базовый URL SE, fileID — UUID файла.
//
// Sentinel errors:
//   - ErrFileNotFound (404) — файл не найден
//   - ErrModeNotAllowed (409) — SE не в режиме edit
//   - ErrFileUploadInProgress (409) — файл в процессе загрузки
func (c *Client) Delete(ctx context.Context, seURL, fileID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/files/%s", normalizeURL(seURL), fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("создание запроса Delete: %w", err)
	}

	// SA-токен для авторизации
	if c.tokenProvider != nil {
		token, tokenErr := c.tokenProvider(ctx)
		if tokenErr != nil {
			return fmt.Errorf("получение токена для SE: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации SE
	if err != nil {
		return fmt.Errorf("запрос Delete к %s: %w", seURL, err)
	}
	defer resp.Body.Close()

	return c.handleDeleteResponse(resp, seURL, fileID)
}

// handleDeleteResponse обрабатывает HTTP-ответ SE после delete.
func (c *Client) handleDeleteResponse(resp *http.Response, seURL, fileID string) error {
	switch resp.StatusCode {
	case http.StatusNoContent:
		c.logger.Debug("Файл удалён из SE",
			slog.String("se_url", seURL),
			slog.String("file_id", fileID),
		)
		return nil

	case http.StatusNotFound:
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("SE вернул 404 (файл не найден)",
			slog.String("se_url", seURL),
			slog.String("file_id", fileID),
			slog.String("body", string(body)),
		)
		return ErrFileNotFound

	case http.StatusConflict:
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		c.logger.Warn("SE вернул 409 (конфликт)",
			slog.String("se_url", seURL),
			slog.String("file_id", fileID),
			slog.String("body", bodyStr),
		)
		// Различаем причину конфликта по коду ошибки в теле ответа
		if strings.Contains(bodyStr, "UPLOAD_IN_PROGRESS") || strings.Contains(bodyStr, "upload") {
			return ErrFileUploadInProgress
		}
		return ErrModeNotAllowed

	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SE %s вернул статус %d при удалении файла %s: %s", seURL, resp.StatusCode, fileID, string(body))
	}
}

// buildMultipartPipe создаёт io.Pipe со streaming multipart body.
// Горутина пишет multipart fields в PipeWriter, HTTP-клиент читает из PipeReader.
func (c *Client) buildMultipartPipe(
	file io.ReadSeeker,
	filename string,
	description string,
	tags []string,
) (*io.PipeReader, *multipart.Writer) {
	pr, pw := io.Pipe()
	mpWriter := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()

		// Поле file (основной файл)
		partWriter, err := mpWriter.CreateFormFile("file", filename)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("создание form file: %w", err))
			return
		}
		if _, err := io.Copy(partWriter, file); err != nil {
			pw.CloseWithError(fmt.Errorf("копирование файла в multipart: %w", err))
			return
		}

		// Поле description (если задано)
		if description != "" {
			if err := mpWriter.WriteField("description", description); err != nil {
				pw.CloseWithError(fmt.Errorf("запись поля description: %w", err))
				return
			}
		}

		// Поле tags (JSON-строка, если задано)
		if len(tags) > 0 {
			tagsJSON, err := json.Marshal(tags)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("маршалинг tags: %w", err))
				return
			}
			if err := mpWriter.WriteField("tags", string(tagsJSON)); err != nil {
				pw.CloseWithError(fmt.Errorf("запись поля tags: %w", err))
				return
			}
		}

		// Закрываем multipart writer (финальная граница)
		if err := mpWriter.Close(); err != nil {
			pw.CloseWithError(fmt.Errorf("закрытие multipart writer: %w", err))
		}
	}()

	return pr, mpWriter
}

// doUploadRequest формирует и выполняет HTTP-запрос загрузки.
func (c *Client) doUploadRequest(
	ctx context.Context,
	seURL string,
	body io.Reader,
	contentType string,
) (*http.Response, error) {
	reqURL := fmt.Sprintf("%s/api/v1/files/upload", normalizeURL(seURL))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("создание запроса Upload: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	// SA-токен для авторизации
	if c.tokenProvider != nil {
		token, tokenErr := c.tokenProvider(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("получение токена для SE: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации SE
	if err != nil {
		return nil, fmt.Errorf("запрос Upload к %s: %w", seURL, err)
	}

	return resp, nil
}

// handleUploadResponse обрабатывает HTTP-ответ SE после upload.
func (c *Client) handleUploadResponse(resp *http.Response, seURL string) (*UploadResult, error) {
	switch resp.StatusCode {
	case http.StatusCreated:
		var result UploadResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("декодирование UploadResult от SE: %w", err)
		}
		c.logger.Debug("Файл загружен в SE",
			slog.String("se_url", seURL),
			slog.String("file_id", result.FileID),
			slog.Int64("size", result.Size),
		)
		return &result, nil

	case http.StatusInsufficientStorage:
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("SE вернул 507 (нет места)",
			slog.String("se_url", seURL),
			slog.String("body", string(body)),
		)
		return nil, ErrStorageFull

	case http.StatusRequestEntityTooLarge:
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("SE вернул 413 (файл слишком большой)",
			slog.String("se_url", seURL),
			slog.String("body", string(body)),
		)
		return nil, ErrFileTooLarge

	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SE %s вернул статус %d: %s", seURL, resp.StatusCode, string(body))
	}
}

// buildTLSConfig создаёт TLS-конфигурацию с кастомным CA-сертификатом.
func buildTLSConfig(caCertPath string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("чтение CA-сертификата: %w", err)
	}

	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		caCertPool = x509.NewCertPool()
	}
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		RootCAs: caCertPool,
	}, nil
}

// normalizeURL убирает trailing slash из URL.
func normalizeURL(rawURL string) string {
	return strings.TrimRight(rawURL, "/")
}
