# zipweather

A Go HTTP service that receives a Brazilian ZIP code (CEP), identifies the corresponding city via ViaCEP, and returns the current temperature in Celsius, Fahrenheit, and Kelvin via WeatherAPI — built with Hexagonal Architecture, containerized with Docker, and deployed on Google Cloud Run.

---

## Live Demo

**Cloud Run URL:** `https://zipweather-295200018580.us-central1.run.app`

```bash
curl "https://zipweather-295200018580.us-central1.run.app/01001000"
```

```json
{"temp_C": 28.5, "temp_F": 83.3, "temp_K": 301.65}
```

---

## Table of Contents

- [Overview](#overview)
- [API Contract](#api-contract)
- [Architecture](#architecture)
- [Running Locally](#running-locally)
- [Running with Docker](#running-with-docker)
- [Running Tests](#running-tests)
- [Makefile](#makefile)
- [Deploying to Cloud Run](#deploying-to-cloud-run)
- [Environment Variables](#environment-variables)

---

## Overview

`zipweather` is a single-endpoint HTTP service. Given a valid 8-digit Brazilian CEP, it:

1. Validates the CEP format (8 numeric digits)
2. Queries [ViaCEP](https://viacep.com.br) to resolve the city name
3. Queries [WeatherAPI](https://www.weatherapi.com) for the current temperature
4. Returns the temperature in three scales: Celsius, Fahrenheit, and Kelvin

The service is written in **Go 1.26.1** using only the standard library (`net/http`, `slog`, `encoding/json`, `context`), follows **Hexagonal Architecture** (Ports & Adapters), and is fully testable without external network calls.

---

## API Contract

### `GET /{cep}` — Weather by CEP

#### Success — `200 OK`

```bash
curl "http://localhost:8080/01001000"
```

```json
{
  "temp_C": 28.5,
  "temp_F": 83.3,
  "temp_K": 301.65
}
```

#### Invalid format — `422 Unprocessable Entity`

Triggered when the CEP does not have exactly 8 numeric digits.

```bash
curl -i "http://localhost:8080/0100100"
```

```
HTTP/1.1 422 Unprocessable Entity

invalid zipcode
```

#### Not found — `404 Not Found`

Triggered when the CEP has correct format but does not exist in ViaCEP's database.

```bash
curl -i "http://localhost:8080/99999999"
```

```
HTTP/1.1 404 Not Found

can not find zipcode
```

### Temperature conversion formulas

```
Fahrenheit = Celsius × 1.8 + 32
Kelvin     = Celsius + 273
```

---

## Architecture

This project follows **Hexagonal Architecture** (Ports & Adapters), making the domain logic completely independent of external APIs and HTTP transport.

```
zipweather/
├── cmd/
│   └── server/
│       └── main.go                      # Composition root: config, wiring, server start
├── internal/
│   ├── domain/
│   │   ├── weather.go                   # Domain types: WeatherResult
│   │   └── errors.go                    # Sentinel errors: ErrNotFound, ErrInvalidCEP
│   ├── ports/
│   │   ├── location.go                  # LocationPort interface
│   │   └── weather.go                   # WeatherPort interface
│   ├── adapters/
│   │   ├── http/
│   │   │   └── handler.go              # Primary adapter: HTTP handler
│   │   ├── viacep/
│   │   │   └── client.go              # Secondary adapter: ViaCEP client
│   │   └── weatherapi/
│   │       └── client.go              # Secondary adapter: WeatherAPI client
│   └── temperature/
│       └── converter.go               # Pure functions: C→F and C→K
├── Dockerfile
├── Makefile
├── .env.example
├── .gitignore
├── go.mod
└── go.sum
```

### Hexagonal layers

| Layer | Responsibility |
|---|---|
| `internal/domain` | Business types and sentinel errors — zero external dependencies |
| `internal/ports` | Port interfaces (`LocationPort`, `WeatherPort`) — contracts between adapters and domain |
| `internal/adapters/http` | Primary adapter: receives HTTP requests, orchestrates ports, writes responses |
| `internal/adapters/viacep` | Secondary adapter: implements `LocationPort` via ViaCEP API |
| `internal/adapters/weatherapi` | Secondary adapter: implements `WeatherPort` via WeatherAPI |
| `internal/temperature` | Pure conversion functions — no I/O, fully unit-testable |
| `cmd/server/main.go` | Composition root: the only place that knows about concrete implementations |

### Dependency rule

```
Adapters  → Ports + Domain
Ports     → Domain
Domain    → nothing (zero external imports)
main      → everything (wiring only)
```

### Design decisions

| Concern | Approach | Reason |
|---|---|---|
| HTTP routing | `net/http` ServeMux (Go 1.22+ patterns) | Stdlib method+path routing, no framework needed |
| Logging | `slog` (stdlib) | Structured JSON logging built into Go 1.21+, Cloud Run compatible |
| Architecture | Hexagonal (Ports & Adapters) | Domain isolated from I/O; fully testable without network |
| Context | `r.Context()` propagated through all I/O | Client disconnects cancel in-flight external API calls |
| External clients | Injected via port interfaces | Testable with `httptest.NewServer`, zero real network calls in tests |
| Error mapping | Sentinel errors in domain → HTTP status in handler | Clean separation between domain and transport |
| Server timeouts | `ReadTimeout`, `WriteTimeout`, `IdleTimeout` | Prevents hung connections in production |
| Docker image | Multi-stage (`golang:1.26-alpine` → `scratch`) | ~10 MB final image, no shell or OS in prod |
| Graceful shutdown | `signal.NotifyContext` + `srv.Shutdown` | Cloud Run sends SIGTERM — in-flight requests must complete |

---

## Running Locally

**Prerequisites:** Go 1.26.1+, a [WeatherAPI](https://www.weatherapi.com) free API key.

```bash
# Clone the repository
git clone https://github.com/henriqueMontalione/zipweather.git
cd zipweather

# Copy and fill environment variables
cp .env.example .env
# Edit .env and set WEATHERAPI_KEY=your_key_here

# Run the server
go run ./cmd/server

# In another terminal, test it
curl "http://localhost:8080/01001000"
```

---

## Running with Docker

```bash
# Build the image
docker build -t zipweather .

# Run the container
docker run -p 8080:8080 \
  -e WEATHERAPI_KEY=your_key_here \
  zipweather

# Test
curl "http://localhost:8080/01001000"
```

---

## Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with race detector (recommended)
go test -race ./...

# Run with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

The test suite covers:

| Package | Test type | What is tested |
|---|---|---|
| `internal/temperature` | Unit | Conversion formulas: C→F and C→K, including edge cases (0°C, 100°C, negatives) |
| `internal/adapters/http` | Integration (`httptest`) | All HTTP scenarios: 200, 404, 422 — with mocked port interfaces |
| `internal/adapters/viacep` | Unit (`httptest.NewServer`) | ViaCEP response parsing, `"erro"` field handling, `ErrNotFound` mapping |
| `internal/adapters/weatherapi` | Unit (`httptest.NewServer`) | WeatherAPI response parsing, city URL-encoding |

No test makes a real network call. External dependencies are replaced by `httptest.NewServer` (for HTTP clients) or mock implementations of the port interfaces (for the handler).

---

## Makefile

| Command | Description |
|---|---|
| `make run` | Start the server locally (`go run ./cmd/server`) |
| `make build` | Compile the binary to `./zipweather` |
| `make test` | Run all tests with race detector |
| `make test-coverage` | Run tests and open HTML coverage report |
| `make lint` | Run `go vet ./...` |
| `make docker-build` | Build the Docker image |
| `make docker-run` | Run the container on port 8080 using `.env` |
| `make clean` | Remove the compiled binary and coverage files |

---

## Deploying to Cloud Run

### Prerequisites

- [Google Cloud SDK](https://cloud.google.com/sdk) installed and authenticated
- A GCP project with Cloud Run API enabled
- A [WeatherAPI](https://www.weatherapi.com) free API key

### Deploy from source (recommended)

Cloud Run can build and deploy directly from source using Cloud Build — no manual image management required:

```bash
# Authenticate and set project
gcloud auth login
gcloud config set project YOUR_PROJECT_ID

# Deploy directly from source
gcloud run deploy zipweather \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars WEATHERAPI_KEY=your_key_here
```

### Deploy from Docker image (alternative)

```bash
# Build and push to Artifact Registry
gcloud artifacts repositories create zipweather \
  --repository-format=docker \
  --location=us-central1

gcloud builds submit \
  --tag us-central1-docker.pkg.dev/YOUR_PROJECT_ID/zipweather/zipweather

gcloud run deploy zipweather \
  --image us-central1-docker.pkg.dev/YOUR_PROJECT_ID/zipweather/zipweather \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars WEATHERAPI_KEY=your_key_here
```

Cloud Run automatically injects the `PORT` variable. The server listens on it and handles `SIGTERM` gracefully.

---

## Environment Variables

| Variable | Description | Required | Default |
|---|---|---|---|
| `PORT` | HTTP server port | No | `8080` |
| `WEATHERAPI_KEY` | API key from weatherapi.com | **Yes** | — |
| `VIACEP_BASE_URL` | ViaCEP base URL (override in tests) | No | `https://viacep.com.br` |
| `WEATHERAPI_BASE_URL` | WeatherAPI base URL (override in tests) | No | `https://api.weatherapi.com` |

Copy `.env.example` to `.env` for local development. The `.env` file is listed in `.gitignore` and must never be committed.
