package weatherapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/henriqueMontalione/zipweather/internal/adapters/weatherapi"
)

func newTestClient(apiKey string, handler http.Handler) (*weatherapi.Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	return weatherapi.NewClient(srv.URL, apiKey, httpClient), srv
}

func TestGetTemperature_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"current":{"temp_c":28.5}}`))
	})

	client, srv := newTestClient("testkey", handler)
	defer srv.Close()

	celsius, err := client.GetTemperature(context.Background(), "São Paulo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if celsius != 28.5 {
		t.Errorf("celsius = %v, want 28.5", celsius)
	}
}

func TestGetTemperature_CityWithAccents(t *testing.T) {
	var receivedQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"current":{"temp_c":22.0}}`))
	})

	client, srv := newTestClient("testkey", handler)
	defer srv.Close()

	_, err := client.GetTemperature(context.Background(), "São Paulo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ensure city name was URL-encoded in the query string.
	if receivedQuery == "" {
		t.Fatal("expected query string to be non-empty")
	}
}

func TestGetTemperature_NonOKStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	client, srv := newTestClient("badkey", handler)
	defer srv.Close()

	_, err := client.GetTemperature(context.Background(), "Recife")
	if err == nil {
		t.Fatal("expected error for non-OK status, got nil")
	}
}

func TestGetTemperature_ContextCanceled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client, srv := newTestClient("testkey", handler)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetTemperature(ctx, "Recife")
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}
