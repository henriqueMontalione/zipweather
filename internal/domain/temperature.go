package domain

// CelsiusToFahrenheit converts Celsius to Fahrenheit: F = C * 1.8 + 32
func CelsiusToFahrenheit(c float64) float64 {
	return c*1.8 + 32
}

// CelsiusToKelvin converts Celsius to Kelvin: K = C + 273
func CelsiusToKelvin(c float64) float64 {
	return c + 273
}
