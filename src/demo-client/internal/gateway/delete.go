package gateway

import (
	"context"
	"fmt"
	"net/http"
)

// DeleteFile удаляет файл через Ingester Module.
// DELETE /upload/api/v1/files/{fileID}?storage_element_id={seID}
func (c *Client) DeleteFile(fileID, storageElementID string) error {
	url := fmt.Sprintf("%s/upload/api/v1/files/%s?storage_element_id=%s",
		c.baseURL, fileID, storageElementID)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("ошибка создания HTTP запроса delete: %w", err)
	}

	resp, err := c.doRequest(req, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return c.handleErrorResponse(resp)
}
