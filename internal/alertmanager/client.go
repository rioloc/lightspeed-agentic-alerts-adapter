package alertmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AlertFetcher interface {
	FetchFiringAlerts(ctx context.Context) ([]Alert, error)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (c *Client) FetchFiringAlerts(ctx context.Context) ([]Alert, error) {
	url := c.baseURL + "/api/v2/alerts?active=true&silenced=false&inhibited=false"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching alerts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var alerts []Alert
	if err := json.Unmarshal(body, &alerts); err != nil {
		return nil, fmt.Errorf("unmarshalling alerts: %w", err)
	}

	return alerts, nil
}
