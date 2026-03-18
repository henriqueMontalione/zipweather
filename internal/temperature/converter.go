package temperature

// CelsiusToFahrenheit converts Celsius to Fahrenheit.
func CelsiusToFahrenheit(c float64) float64 {
	return c*1.8 + 32
}

// CelsiusToKelvin converts Celsius to Kelvin.
func CelsiusToKelvin(c float64) float64 {
	return c + 273
}
