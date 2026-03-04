package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetMetadata получает метаданные файла через Query Module.
// GET /query/api/v1/files/{fileID}
func (c *Client) GetMetadata(fileID string) (*FileInfo, error) {
	url := c.baseURL + "/query/api/v1/files/" + fileID
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}

	resp, err := c.doRequest(req, false)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}
	defer resp.Body.Close()

	var result FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа metadata: %w", err)
	}

	return &result, nil
}
