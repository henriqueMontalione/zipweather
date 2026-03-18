# CLAUDE.md

## Papel e contexto

Você está implementando um serviço HTTP em Go que, dado um CEP brasileiro, retorna a temperatura atual da cidade correspondente em Celsius, Fahrenheit e Kelvin. O sistema usa **arquitetura hexagonal** e é publicado no Google Cloud Run. Siga estas instruções à risca em toda interação com este projeto.

**Stack:**
- Go 1.26.1
- stdlib: `net/http`, `slog`, `encoding/json`, `context`, `os`, `errors`, `os/signal`
- Docker multi-stage: `golang:1.26-alpine` → `scratch`
- APIs externas: ViaCEP (localização) e WeatherAPI (temperatura)

---

## Regras de código — NUNCA faça

- Nunca use `panic` em código de aplicação
- Nunca ignore erros — trate todos explicitamente
- Nunca hardcode valores de configuração (API keys, URLs externas, porta) — use variáveis de ambiente
- Nunca adicione comentários que apenas repetem o que o código já diz
- Nunca escreva lógica de negócio dentro dos adapters (handlers ou clientes HTTP)
- Nunca escreva lógica de conversão de temperatura fora do pacote `internal/temperature`
- Nunca faça chamadas HTTP diretamente no handler — use os ports (interfaces em `internal/ports/`)
- Nunca use `http.DefaultClient` sem timeout explícito — chamadas externas podem travar goroutines
- Nunca propague erros internos ao cliente — retorne apenas as mensagens do contrato (`invalid zipcode`, `can not find zipcode`)
- Nunca use `context.Background()` nos adapters secundários — propague sempre o context recebido do caller
- Nunca adicione dependências externas sem necessidade clara — prefira stdlib
- Nunca use Gin ou qualquer outro framework HTTP — o projeto usa apenas `net/http` stdlib
- Nunca comite o arquivo `.env` — ele contém credenciais reais e está no `.gitignore`

---

## Arquitetura Hexagonal

Este projeto segue a **Arquitetura Hexagonal** (Ports & Adapters), de Alistair Cockburn.

### Conceitos aplicados

| Conceito | O que é | Onde está |
|---|---|---|
| **Domínio** | Tipos de negócio e erros sentinela, sem I/O | `internal/domain/` |
| **Port** | Interface que define o contrato entre adapters e domínio | `internal/ports/` |
| **Adapter primário** | Dirige a aplicação (HTTP handler) | `internal/adapters/http/` |
| **Adapter secundário** | Dirigido pela aplicação (ViaCEP, WeatherAPI) | `internal/adapters/viacep/`, `internal/adapters/weatherapi/` |
| **Composition root** | Instancia e conecta tudo | `cmd/server/main.go` |

### Regra de dependência

```
Adapters  → Ports + Domain
Ports     → Domain
Domain    → nada (zero imports externos)
main      → tudo (único lugar com wiring)
```

O domínio nunca importa ports nem adapters. Os ports importam apenas o domínio (para os tipos). Os adapters importam ports e domínio. `main.go` é o único lugar onde implementações concretas são instanciadas.

### Estrutura de pastas

```
zipweather/
├── cmd/
│   └── server/
│       └── main.go                      # Composition root: config, wiring, start
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
│       └── converter.go               # Pure functions: CelsiusToFahrenheit, CelsiusToKelvin
├── Dockerfile
├── Makefile
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── CLAUDE.md
├── README.md
└── IMPLEMENTATION.md
```

### Ports (interfaces)

Definidos em `internal/ports/`. Os adapters secundários implementam; o adapter primário consome. Ambos importam `ports/` — nunca um ao outro:

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

### Erros de domínio

Definidos em `internal/domain/errors.go`. Retornados pelos adapters secundários e mapeados para status HTTP pelo handler:

```go
var ErrNotFound    = errors.New("can not find zipcode")
var ErrInvalidCEP  = errors.New("invalid zipcode")
```

Mapeamento no handler:
- `ErrInvalidCEP` → 422
- `ErrNotFound` → 404
- qualquer outro → 500 (sem expor detalhes ao cliente)

### Contrato da API

| Cenário | Status | Body |
|---|---|---|
| Sucesso | 200 | `{"temp_C": 28.5, "temp_F": 83.3, "temp_K": 301.65}` |
| CEP inválido (formato) | 422 | `invalid zipcode` |
| CEP não encontrado | 404 | `can not find zipcode` |

### Fórmulas de conversão (exatas, conforme requisito)

```
Fahrenheit = Celsius * 1.8 + 32
Kelvin     = Celsius + 273
```

### Fluxo de dados

```
GET /{cep}
  └─ handler.GetWeather(w, r)                          [adapter primário]
        ├─ valida CEP → 422 "invalid zipcode"
        ├─ ctx = r.Context()                            [propaga context do request]
        ├─ ports.LocationPort.GetLocation(ctx, cep)     [chama port]
        │     └─ ViaCEPClient.GetLocation(ctx, cep)    [adapter secundário executa]
        │           ├─ GET viacep.com.br/ws/{cep}/json/
        │           ├─ "erro": true → domain.ErrNotFound → 404
        │           └─ retorna city name
        ├─ ports.WeatherPort.GetTemperature(ctx, city)  [chama port]
        │     └─ WeatherAPIClient.GetTemperature(...)   [adapter secundário executa]
        │           └─ GET api.weatherapi.com/v1/current.json
        └─ temperature.Convert(celsius)                 [lógica pura]
              └─ json.NewEncoder(w).Encode(result) → 200

```

---

## Context — uso obrigatório

O `context.Context` é a espinha dorsal de cancelamento neste serviço. Regras:

- **Handler:** extrai o context do request com `r.Context()` e o passa para todos os ports
- **Adapters secundários:** recebem `ctx` como primeiro parâmetro e o passam para `http.NewRequestWithContext(ctx, ...)`
- **Benefício:** se o cliente HTTP desconectar durante uma chamada ao ViaCEP ou WeatherAPI, a requisição é cancelada imediatamente — sem goroutine leak
- **Graceful shutdown:** `signal.NotifyContext` no `main.go` cancela o context no SIGTERM, e `http.Server.Shutdown` drena conexões em andamento

```go
// handler.go
func (h *Handler) GetWeather(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()  // nunca criar context.Background() aqui
    city, err := h.location.GetLocation(ctx, cep)
    ...
}

// viacep/client.go
func (c *Client) GetLocation(ctx context.Context, cep string) (string, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    ...
}
```

---

## Convenções obrigatórias

### Nomenclatura
- Ports (interfaces): sufixo `Port` — `LocationPort`, `WeatherPort`
- Adapters: nome do serviço — `ViaCEPClient`, `WeatherAPIClient`
- Construtores: sempre `New{Type}` — `NewViaCEPClient`, `NewWeatherAPIClient`, `NewHandler`
- Métodos: verbos claros — `GetLocation`, `GetTemperature`

### Go idiomático
- Use `net/http` ServeMux nativo com path patterns do Go 1.22+: `"GET /{cep}"`
- Use `slog` (stdlib) para logging estruturado — nunca `fmt.Println` em produção
- Use `encoding/json` da stdlib para serialização
- Prefira interfaces pequenas (1–2 métodos)
- Siga Effective Go, Go Code Review Comments e Google Go Style Guide

### Configuração via variáveis de ambiente

| Variável | Descrição | Default |
|---|---|---|
| `PORT` | Porta HTTP do servidor | `8080` |
| `WEATHERAPI_KEY` | API key da WeatherAPI.com | (obrigatório) |
| `VIACEP_BASE_URL` | Base URL da ViaCEP (injetável em testes) | `https://viacep.com.br` |
| `WEATHERAPI_BASE_URL` | Base URL da WeatherAPI (injetável em testes) | `https://api.weatherapi.com` |

---

## Checklist antes de cada commit

- [ ] Todos os erros estão sendo tratados
- [ ] Nenhum valor hardcoded — tudo vem de variáveis de ambiente
- [ ] `context.Context` propagado: handler usa `r.Context()`, adapters usam `http.NewRequestWithContext`
- [ ] Domínio não importa nenhum adapter ou port — regra de dependência hexagonal respeitada
- [ ] Handler conhece apenas os ports — nunca implementações concretas
- [ ] `temperature/` não tem dependências externas nem de I/O
- [ ] Testes adicionados ou atualizados para a feature
- [ ] Testes do handler usam `httptest.NewRecorder` e mocks dos ports
- [ ] Testes de `temperature/` são unitários puros (sem I/O, sem mocks)
- [ ] `go vet ./...` e `go build ./...` sem erros
- [ ] `go mod tidy` rodado
- [ ] `.env` não está sendo commitado
- [ ] `docker build` funciona e `docker run` executa corretamente

---

## Git workflow

### Branches
- Crie uma branch por feature a partir da `main`
- Nomenclatura: `feat/`, `fix/`, `test/`, `chore/`, `docs/`
- Após aprovação do usuário, faça merge na `main`
- A próxima branch sempre parte da `main` atualizada

### Fluxo
1. `git checkout main`
2. `git checkout -b feat/nome-da-feature`
3. Implemente em commits atômicos
4. Adicione ou atualize os testes da feature antes de commitar
5. Apresente os arquivos ao usuário para aprovação
6. `git add <arquivos específicos>` — nunca `git add .`
7. Commit após aprovação explícita do usuário
8. `git push -u origin feat/nome-da-feature`
9. Merge na `main`
10. `git push origin main`

### Commits
- Mensagens em inglês, Conventional Commits
- `feat:` `fix:` `test:` `chore:` `docs:`
- Um commit = uma mudança lógica
- Nunca mencionar Claude ou IA na mensagem

---

## Notas críticas de implementação

### Validação do CEP
Validação no handler antes de chamar qualquer port. CEP deve ter exatamente 8 dígitos numéricos:

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

### Cliente HTTP com timeout
Nunca use `http.DefaultClient`. Instancie um client com timeout explícito e injete via construtor:

```go
client := &http.Client{Timeout: 10 * time.Second}
```

### Timeouts no http.Server
O servidor HTTP deve ter timeouts explícitos para evitar conexões presas. O `WriteTimeout` deve ser maior que o timeout dos clients externos:

```go
srv := &http.Server{
    Addr:         ":" + port,
    Handler:      mux,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 30 * time.Second,
    IdleTimeout:  120 * time.Second,
}
```

### Detecção de CEP inexistente no ViaCEP
A API ViaCEP retorna HTTP 200 mesmo para CEPs inexistentes, com `{"erro": "true"}` no body:

```go
type viaCEPResponse struct {
    Localidade string `json:"localidade"`
    Erro       string `json:"erro"`
}
// se resp.Erro != "" → retornar domain.ErrNotFound
```

### Encoding de cidade na WeatherAPI
Cidades com acentos ou espaços devem ser URL-encoded antes de compor a query string:

```go
url.QueryEscape(city)
```

### Response HTTP com mensagem de erro em texto puro
O contrato define body de erro como texto simples, não JSON:

```go
http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
http.Error(w, "can not find zipcode", http.StatusNotFound)
```

### Dockerfile multi-stage para Cloud Run
- Stage `builder`: `golang:1.26-alpine` — compila com `CGO_ENABLED=0 GOOS=linux -ldflags="-s -w"`
- Stage `runner`: `scratch` — apenas o binário e `ca-certificates`
- Use `CMD` (não `ENTRYPOINT`) para compatibilidade com Cloud Run

### Porta no Cloud Run
Cloud Run injeta a porta via `PORT`. O servidor deve ouvir nela:

```go
port := os.Getenv("PORT")
if port == "" {
    port = "8080"
}
```

### Graceful shutdown obrigatório para Cloud Run
Cloud Run envia `SIGTERM` antes de encerrar instâncias. Sem graceful shutdown, requests em andamento são abortados:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

go func() {
    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    srv.Shutdown(shutdownCtx)
}()

srv.ListenAndServe()
```

---

## Dependências do projeto

```
github.com/joho/godotenv  — carregamento do .env em desenvolvimento local (apenas main.go)
```

Minimize dependências. Sem frameworks HTTP (Gin, Echo, Chi etc.) — o projeto usa apenas `net/http` stdlib. Finalize sempre com `go mod tidy`.
