package gateway

import "time"

// --- Общие модели ответов API ---

// ErrorResponse — стандартный формат ошибки от API Gateway.
type ErrorResponse struct {
	Error *ErrorDetail `json:"error"`
}

// ErrorDetail — детали ошибки API.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- Модели Upload (Ingester Module) ---

// UploadResult — результат загрузки файла от Ingester Module.
// POST /upload/api/v1/files/upload
type UploadResult struct {
	FileID           string    `json:"file_id"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	Size             int64     `json:"size"`
	Checksum         string    `json:"checksum"`
	UploadedBy       string    `json:"uploaded_by"`
	UploadedAt       time.Time `json:"uploaded_at"`
	Description      *string   `json:"description,omitempty"`
	Tags             []string  `json:"tags"`
	Status           string    `json:"status"`
	StorageElementID string    `json:"storage_element_id"`
	RetentionPolicy  string    `json:"retention_policy"`
	TTLDays          *int      `json:"ttl_days,omitempty"`
	ExpiresAt        *string   `json:"expires_at,omitempty"`
}

// UploadParams — параметры для загрузки файла.
type UploadParams struct {
	Description     string   // Описание файла (опционально)
	Tags            []string // Теги файла (опционально)
	RetentionPolicy string   // "temporary" или "permanent" (default: "temporary")
	TTLDays         int      // Срок хранения в днях для temporary (1-365, default: 30)
}

// --- Модели Search (Query Module) ---

// SearchRequest — параметры поиска файлов.
// POST /query/api/v1/search
type SearchRequest struct {
	Query           string   `json:"query,omitempty"`
	Filename        string   `json:"filename,omitempty"`
	FileExtension   string   `json:"file_extension,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	UploadedBy      string   `json:"uploaded_by,omitempty"`
	RetentionPolicy string   `json:"retention_policy,omitempty"`
	Status          string   `json:"status,omitempty"`
	MinSize         *int64   `json:"min_size,omitempty"`
	MaxSize         *int64   `json:"max_size,omitempty"`
	UploadedAfter   string   `json:"uploaded_after,omitempty"`
	UploadedBefore  string   `json:"uploaded_before,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Offset          int      `json:"offset,omitempty"`
	SortBy          string   `json:"sort_by,omitempty"`
	SortOrder       string   `json:"sort_order,omitempty"`
}

// SearchResponse — результат поиска от Query Module.
type SearchResponse struct {
	Items   []FileInfo `json:"items"`
	Total   int        `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	HasMore bool       `json:"has_more"`
}

// FileInfo — информация о файле (используется в результатах поиска и метаданных).
type FileInfo struct {
	FileID           string    `json:"file_id"`
	OriginalFilename string    `json:"original_filename"`
	ContentType      string    `json:"content_type"`
	Size             int64     `json:"size"`
	Checksum         string    `json:"checksum"`
	UploadedBy       string    `json:"uploaded_by"`
	UploadedAt       time.Time `json:"uploaded_at"`
	Description      *string   `json:"description,omitempty"`
	Tags             []string  `json:"tags"`
	Status           string    `json:"status"`
	RetentionPolicy  string    `json:"retention_policy"`
	TTLDays          *int      `json:"ttl_days,omitempty"`
	ExpiresAt        *string   `json:"expires_at,omitempty"`
	StorageElementID string    `json:"storage_element_id"`
	SEMode           string    `json:"se_mode"`
}

// --- Модели Download ---

// DownloadResponse — результат запроса на скачивание файла.
// Содержит reader для потоковой передачи и метаданные ответа.
type DownloadResponse struct {
	// Body — поток данных файла (вызывающий обязан закрыть).
	Body interface {
		Read([]byte) (int, error)
		Close() error
	}
	// ContentType — MIME-тип файла.
	ContentType string
	// ContentLength — размер файла в байтах.
	ContentLength int64
	// Filename — имя файла из Content-Disposition.
	Filename string
	// ETag — контрольная сумма файла.
	ETag string
}

// --- Модели Health ---

// HealthStatus — статус здоровья backend-сервиса.
type HealthStatus struct {
	// Status — общий статус ("ok", "degraded", "fail").
	Status string `json:"status"`
	// Ready — сервис готов к работе.
	Ready bool
}
