package weatherapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type weatherAPIResponse struct {
	Current struct {
		TempC float64 `json:"temp_c"`
	} `json:"current"`
}

// Client implements ports.WeatherPort via the WeatherAPI.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new WeatherAPI client.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, apiKey: apiKey, httpClient: httpClient}
}

// GetTemperature returns the current temperature in Celsius for a given city.
func (c *Client) GetTemperature(ctx context.Context, city string) (float64, error) {
	q := url.QueryEscape(city)
	apiURL := fmt.Sprintf("%s/v1/current.json?key=%s&q=%s&aqi=no", c.baseURL, c.apiKey, q)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return 0, fmt.Errorf("weatherapi: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("weatherapi: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("weatherapi: unexpected status %d", resp.StatusCode)
	}

	var data weatherAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("weatherapi: decode response: %w", err)
	}

	return data.Current.TempC, nil
}
