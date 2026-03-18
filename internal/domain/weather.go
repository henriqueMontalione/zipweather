package domain

// WeatherResult holds the temperature in three scales.
type WeatherResult struct {
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}
