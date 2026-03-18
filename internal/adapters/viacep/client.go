package viacep

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/henriqueMontalione/zipweather/internal/domain"
)

type viaCEPResponse struct {
	Localidade string `json:"localidade"`
	Erro       string `json:"erro"`
}

// Client implements ports.LocationPort via the ViaCEP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new ViaCEP client.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// GetLocation resolves a CEP to a city name using the ViaCEP API.
func (c *Client) GetLocation(ctx context.Context, cep string) (string, error) {
	url := fmt.Sprintf("%s/ws/%s/json/", c.baseURL, cep)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("viacep: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("viacep: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", domain.ErrNotFound
	}

	var data viaCEPResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("viacep: decode response: %w", err)
	}

	if data.Erro != "" {
		return "", domain.ErrNotFound
	}

	if data.Localidade == "" {
		return "", domain.ErrNotFound
	}

	return data.Localidade, nil
}
