package ports

import "context"

// LocationPort is the contract for resolving a CEP to a city name.
type LocationPort interface {
	GetLocation(ctx context.Context, cep string) (city string, err error)
}
