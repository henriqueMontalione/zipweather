package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	httphandler "github.com/henriqueMontalione/zipweather/internal/adapters/http"
	"github.com/henriqueMontalione/zipweather/internal/adapters/viacep"
	"github.com/henriqueMontalione/zipweather/internal/adapters/weatherapi"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Load .env in local development — ignore error in production (Cloud Run injects env vars).
	_ = godotenv.Load()

	apiKey := os.Getenv("WEATHERAPI_KEY")
	if apiKey == "" {
		slog.Error("WEATHERAPI_KEY is required")
		os.Exit(1)
	}

	viacepBaseURL := os.Getenv("VIACEP_BASE_URL")
	if viacepBaseURL == "" {
		viacepBaseURL = "https://viacep.com.br"
	}

	weatherAPIBaseURL := os.Getenv("WEATHERAPI_BASE_URL")
	if weatherAPIBaseURL == "" {
		weatherAPIBaseURL = "https://api.weatherapi.com"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	locAdapter := viacep.NewClient(viacepBaseURL, httpClient)
	wthrAdapter := weatherapi.NewClient(weatherAPIBaseURL, apiKey, httpClient)
	h := httphandler.NewHandler(locAdapter, wthrAdapter)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{cep}", h.GetWeather)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
	}()

	slog.Info("server started", "port", port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
