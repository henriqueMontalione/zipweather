package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/henriqueMontalione/zipweather/internal/domain"
	"github.com/henriqueMontalione/zipweather/internal/ports"
)

// Handler is the primary adapter. It handles HTTP requests and delegates
// to port interfaces — it never depends on concrete implementations.
type Handler struct {
	location ports.LocationPort
	weather  ports.WeatherPort
}

// NewHandler creates a new Handler with the given port implementations.
func NewHandler(loc ports.LocationPort, wthr ports.WeatherPort) *Handler {
	return &Handler{location: loc, weather: wthr}
}

// GetWeather handles GET /{cep} — validates the CEP, resolves city, fetches temperature.
func (h *Handler) GetWeather(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cep := r.PathValue("cep")

	if !isValidCEP(cep) {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	city, err := h.location.GetLocation(ctx, cep)
	if err != nil {
		writeError(ctx, w, err)
		return
	}

	celsius, err := h.weather.GetTemperature(ctx, city)
	if err != nil {
		writeError(ctx, w, err)
		return
	}

	result := domain.WeatherResult{
		TempC: celsius,
		TempF: domain.CelsiusToFahrenheit(celsius),
		TempK: domain.CelsiusToKelvin(celsius),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.ErrorContext(ctx, "encode response", "err", err)
	}
}

func writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		http.Error(w, "can not find zipcode", http.StatusNotFound)
	default:
		slog.ErrorContext(ctx, "internal error", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func isValidCEP(cep string) bool {
	if len(cep) != 8 {
		return false
	}
	for _, c := range cep {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
