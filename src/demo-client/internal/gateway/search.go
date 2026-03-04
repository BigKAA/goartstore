package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Search выполняет поиск файлов через Query Module.
// POST /query/api/v1/search (JSON body)
func (c *Client) Search(params SearchRequest) (*SearchResponse, error) {
	// Сериализуем параметры поиска в JSON.
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации search request: %w", err)
	}

	url := c.baseURL + "/query/api/v1/search"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(req, false)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}
	defer resp.Body.Close()

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа search: %w", err)
	}

	return &result, nil
}
