package ports

import "context"

// WeatherPort is the contract for fetching the current temperature of a city.
type WeatherPort interface {
	GetTemperature(ctx context.Context, city string) (celsius float64, err error)
}
