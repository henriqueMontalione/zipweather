package domain_test

import (
	"testing"

	"github.com/henriqueMontalione/zipweather/internal/domain"
)

func TestCelsiusToFahrenheit(t *testing.T) {
	tests := []struct {
		celsius float64
		want    float64
	}{
		{0, 32},
		{100, 212},
		{-40, -40},
		{28.5, 83.3},
	}
	for _, tt := range tests {
		got := domain.CelsiusToFahrenheit(tt.celsius)
		if got != tt.want {
			t.Errorf("CelsiusToFahrenheit(%v) = %v, want %v", tt.celsius, got, tt.want)
		}
	}
}

func TestCelsiusToKelvin(t *testing.T) {
	tests := []struct {
		celsius float64
		want    float64
	}{
		{0, 273},
		{100, 373},
		{-273, 0},
		{28.5, 301.5},
	}
	for _, tt := range tests {
		got := domain.CelsiusToKelvin(tt.celsius)
		if got != tt.want {
			t.Errorf("CelsiusToKelvin(%v) = %v, want %v", tt.celsius, got, tt.want)
		}
	}
}
