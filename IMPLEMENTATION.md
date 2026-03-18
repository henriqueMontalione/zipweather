# Implementation Notes

Detailed explanation of each component, the decisions made, and the reasoning behind them.

---

## Hexagonal Architecture — visão geral

Este projeto segue a **Arquitetura Hexagonal** de Alistair Cockburn. A ideia central é isolar o domínio (regras de negócio) de tudo que é I/O — HTTP, APIs externas, banco de dados.

```
  ┌─────────────────────────────────────────────────┐
  │                    ADAPTERS                      │
  │  ┌──────────┐               ┌────────────────┐  │
  │  │  HTTP    │               │    ViaCEP      │  │
  │  │ Handler  │               │  WeatherAPI    │  │
  │  │(primary) │               │  (secondary)   │  │
  │  └────┬─────┘               └───────┬────────┘  │
  └───────│───────────────────────────────│──────────┘
          │           ┌─────────┐         │
          └──────────►│  PORTS  │◄────────┘
                      └────┬────┘
                           │
                    ┌──────┴──────┐
                    │   DOMAIN    │
                    └─────────────┘
```

A regra de dependência:
- Adapters importam `ports/` e `domain/`
- `ports/` importa apenas `domain/`
- `domain/` não importa nada externo

---

## internal/domain/

O núcleo da aplicação. Sem imports de `net/http`, sem I/O, sem dependências externas — nem mesmo do pacote `ports/`.

### weather.go

Tipo de domínio usado como resposta da API:

```go
type WeatherResult struct {
    TempC float64 `json:"temp_C"`
    TempF float64 `json:"temp_F"`
    TempK float64 `json:"temp_K"`
}
```

### errors.go

Erros sentinela do domínio. Definidos aqui, retornados pelos adapters secundários, mapeados para status HTTP pelo adapter primário:

```go
var ErrNotFound   = errors.New("can not find zipcode")
var ErrInvalidCEP = errors.New("invalid zipcode")
```

**Por que erros sentinela?**

O handler usa `errors.Is(err, domain.ErrNotFound)` para mapear para 404 sem string matching frágil. Cada erro tem semântica clara, testável, e não vaza detalhes de implementação para o cliente.

---

## internal/ports/

Define os **ports** — as interfaces que descrevem o que a aplicação precisa do mundo externo. É um pacote separado do `domain/` para tornar a arquitetura hexagonal explícita e visível na estrutura de pastas.

O handler (adapter primário) consome esses ports. Os adapters secundários os implementam. Nenhum dos dois se conhece diretamente.

```go
// internal/ports/location.go
type LocationPort interface {
    GetLocation(ctx context.Context, cep string) (city string, err error)
}

// internal/ports/weather.go
type WeatherPort interface {
    GetTemperature(ctx context.Context, city string) (celsius float64, err error)
}
```

**Por que `context.Context` nos ports?**

Os ports são a fronteira entre o domínio e o mundo externo. Toda operação que pode ser lenta ou cancelada deve carregar o context. Isso garante que o cancelamento iniciado no handler (quando o cliente HTTP desconecta ou o Cloud Run envia SIGTERM) se propague até as chamadas às APIs externas.

**Por que `ports/` é um pacote separado de `domain/`?**

Separar tipos de domínio das interfaces deixa a estrutura autoexplicativa: quem abrir o repositório vê `domain/`, `ports/` e `adapters/` e entende imediatamente o padrão hexagonal sem precisar ler documentação.

---

## internal/temperature/converter.go

Funções puras, zero I/O, zero estado. Conversão de temperatura é regra de negócio, não lógica de transporte. Isolada aqui, é testável sem nenhuma infraestrutura.

```go
func CelsiusToFahrenheit(c float64) float64 {
    return c*1.8 + 32
}

func CelsiusToKelvin(c float64) float64 {
    return c + 273
}
```

**Testes:** table-driven, cobrindo valores conhecidos (0°C, 100°C, negativos), sem mocks, sem I/O:

```go
func TestCelsiusToFahrenheit(t *testing.T) {
    tests := []struct {
        celsius float64
        want    float64
    }{
        {0, 32},
        {100, 212},
        {-40, -40},
    }
    for _, tt := range tests {
        got := CelsiusToFahrenheit(tt.celsius)
        if got != tt.want {
            t.Errorf("CelsiusToFahrenheit(%v) = %v, want %v", tt.celsius, got, tt.want)
        }
    }
}
```

---

## internal/adapters/viacep/client.go

**Adapter secundário** que implementa `ports.LocationPort`.

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client
```

O `baseURL` e o `*http.Client` são injetados via construtor. Em testes, `baseURL` aponta para um `httptest.NewServer` — nenhuma chamada real ao ViaCEP ocorre.

**Comportamento especial do ViaCEP:**

A API retorna HTTP 200 mesmo para CEPs inexistentes. O sinal de "não encontrado" é o campo `"erro": "true"` no JSON:

```go
type viaCEPResponse struct {
    Localidade string `json:"localidade"`
    Erro       string `json:"erro"`
}
// se resp.Erro != "" → retornar domain.ErrNotFound
```

**Context propagation:**

```go
func (c *Client) GetLocation(ctx context.Context, cep string) (string, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    // se o cliente HTTP desconectar, ctx é cancelado e esta chamada aborta
}
```

---

## internal/adapters/weatherapi/client.go

**Adapter secundário** que implementa `ports.WeatherPort`.

```go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client
```

**Parsing da resposta:**

Apenas os campos necessários são extraídos — sem over-fetching:

```go
type weatherAPIResponse struct {
    Current struct {
        TempC float64 `json:"temp_c"`
    } `json:"current"`
}
```

**Encoding da cidade:**

Nomes do ViaCEP podem ter acentos (`São Paulo`) ou espaços. URL-encoding é obrigatório:

```go
q := url.QueryEscape(city)
apiURL := fmt.Sprintf("%s/v1/current.json?key=%s&q=%s&aqi=no", c.baseURL, c.apiKey, q)
```

---

## internal/adapters/http/handler.go

**Adapter primário.** Recebe requisições HTTP, valida o CEP, chama os ports, e escreve a resposta.

**Struct com ports injetados:**

```go
type Handler struct {
    location ports.LocationPort
    weather  ports.WeatherPort
}

func NewHandler(loc ports.LocationPort, wthr ports.WeatherPort) *Handler
```

O handler conhece apenas as interfaces dos ports — nunca os clientes concretos. Isso é o coração da arquitetura hexagonal: o adapter primário depende de abstrações, não de implementações.

**Validação do CEP:**

```go
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
```

**Context do request:**

```go
func (h *Handler) GetWeather(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // propaga cancelamento de cliente e SIGTERM
    cep := r.PathValue("cep")

    if !isValidCEP(cep) {
        http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
        return
    }

    city, err := h.location.GetLocation(ctx, cep)
    ...
    celsius, err := h.weather.GetTemperature(ctx, city)
    ...
}
```

**Mapeamento de erros:**

```go
switch {
case errors.Is(err, domain.ErrNotFound):
    http.Error(w, "can not find zipcode", http.StatusNotFound)
    return
default:
    slog.ErrorContext(ctx, "internal error", "err", err)
    http.Error(w, "internal server error", http.StatusInternalServerError)
    return
}
```

Erros internos são logados com `slog` mas nunca repassados ao cliente.

**Mensagens de erro em texto puro:**

O contrato define body de erro como texto simples, não JSON. `http.Error` define `Content-Type: text/plain` automaticamente:

```go
http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
```

**Testes do handler com mocks dos ports:**

```go
type mockLocation struct {
    city string
    err  error
}

func (m *mockLocation) GetLocation(_ context.Context, _ string) (string, error) {
    return m.city, m.err
}

func TestGetWeather_Success(t *testing.T) {
    h := NewHandler(
        &mockLocation{city: "São Paulo"},
        &mockWeather{celsius: 28.5},
    )
    req := httptest.NewRequest(http.MethodGet, "/01001000", nil)
    req.SetPathValue("cep", "01001000")
    w := httptest.NewRecorder()

    h.GetWeather(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("expected status 200, got %d", w.Code)
    }

    var result domain.WeatherResult
    if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
        t.Fatalf("failed to decode response: %v", err)
    }
    if result.TempC != 28.5 {
        t.Errorf("temp_C = %v, want 28.5", result.TempC)
    }
}
```

---

## cmd/server/main.go

**Composition root.** O único arquivo que conhece implementações concretas e faz o wiring.

**Leitura de config:**

```go
apiKey := os.Getenv("WEATHERAPI_KEY")
if apiKey == "" {
    slog.Error("WEATHERAPI_KEY is required")
    os.Exit(1)
}
```

**Wiring:**

```go
httpClient := &http.Client{Timeout: 10 * time.Second}

locAdapter  := viacep.NewClient(viacepBaseURL, httpClient)
wthrAdapter := weatherapi.NewClient(weatherAPIBaseURL, apiKey, httpClient)
h           := handler.NewHandler(locAdapter, wthrAdapter)

mux := http.NewServeMux()
mux.HandleFunc("GET /{cep}", h.GetWeather)
```

**Timeouts no servidor HTTP:**

O `http.Server` padrão não tem timeouts, o que pode causar goroutines presas em conexões lentas. O `WriteTimeout` deve ser maior que o timeout dos clients externos para acomodar chamadas às APIs:

```go
srv := &http.Server{
    Addr:         ":" + port,
    Handler:      mux,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 30 * time.Second,
    IdleTimeout:  120 * time.Second,
}
```

**Graceful shutdown:**

Cloud Run envia `SIGTERM` com até 10 segundos de tolerância antes de matar o processo. Sem graceful shutdown, requests em andamento são abortados abruptamente:

```go
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
```

**Logging estruturado:**

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
```

JSON handler em produção — Cloud Run faz parse de logs JSON e os exibe estruturados no Cloud Logging.

---

## Dockerfile

Multi-stage build para imagem mínima no Cloud Run.

**Stage 1 — builder (`golang:1.26-alpine`)**

- `CGO_ENABLED=0`: binário estático, sem dependência de libc
- `GOOS=linux`: cross-compila para Linux mesmo em macOS
- `-ldflags="-s -w"`: remove símbolos de debug e DWARF, reduzindo o binário em ~30%

**Stage 2 — runner (`scratch`)**

- `scratch` é imagem vazia — sem shell, sem OS, apenas o binário
- `ca-certificates.crt` copiado do builder — necessário para HTTPS funcionar no container
- `CMD` (não `ENTRYPOINT`) — compatível com Cloud Run, que injeta `PORT` via variável de ambiente

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/server /server
CMD ["/server"]
```

Tamanho final da imagem: ~10 MB.

---

## Context — fluxo completo

```
Cloud Run SIGTERM
  └─ signal.NotifyContext cancela ctx do main
        └─ srv.Shutdown(10s) — drena conexões ativas antes de encerrar

Cliente HTTP desconecta durante chamada ao ViaCEP
  └─ r.Context() é cancelado automaticamente pelo net/http
        └─ handler passa ctx para LocationPort.GetLocation
              └─ http.NewRequestWithContext aborta a chamada ao ViaCEP
                    └─ retorna ctx.Err() imediatamente — sem goroutine leak
```

O context garante que recursos não sejam desperdiçados em trabalho cujo resultado ninguém vai consumir.

---

## Arquivos de suporte

### .gitignore

```
.env
zipweather
coverage.out
```

### .env.example

```
PORT=8080
WEATHERAPI_KEY=your_weatherapi_key_here
VIACEP_BASE_URL=https://viacep.com.br
WEATHERAPI_BASE_URL=https://api.weatherapi.com
```

---

## External APIs

### ViaCEP

- **Endpoint:** `GET https://viacep.com.br/ws/{cep}/json/`
- **Sucesso:** HTTP 200, campo `localidade` com o nome da cidade
- **Não encontrado:** HTTP 200, campo `"erro": "true"` no body
- **Formato inválido:** HTTP 400 (não alcançado — validação ocorre no handler antes da chamada)

### WeatherAPI

- **Endpoint:** `GET https://api.weatherapi.com/v1/current.json?key={KEY}&q={city}&aqi=no`
- **Sucesso:** HTTP 200, campo `current.temp_c`
- **Free tier:** 1 milhão de chamadas/mês — suficiente para produção de portfolio
