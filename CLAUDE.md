# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Objetivo del proyecto

Port a Go de la librería TypeScript [`kindle-api`](https://github.com/transitive-bullshit/kindle-api) (referencia local en `kindle-api/`). La librería es un cliente no oficial para la API privada de Amazon Kindle que permite acceder a la biblioteca de libros, metadatos y contenido de lectura.

## Comandos principales

```bash
go build ./...
go test ./...
go test -run TestNombre ./paquete/...   # test individual
go test -v -race ./...                 # con detector de race conditions
go vet ./...
golangci-lint run                      # linter (requiere instalación)
```

## Arquitectura planeada

### Estructura de paquetes

```
kindle-go/
├── kindle.go              # Client struct, constructor, métodos públicos
├── types.go               # Tipos, interfaces y constantes del dominio
├── options.go             # Opciones del constructor con functional options pattern
├── internal/
│   └── tlsproxy/          # Lógica del proxy TLS (comunicación con tls-client-api)
├── examples/
│   └── basic/main.go      # Ejemplo de uso
└── go.mod
```

### Componentes clave

**`Client`** — struct principal con estado de sesión:
- `books []Book` — biblioteca cargada tras `Init()`
- `sessionID`, `adpSessionToken` — tokens de sesión Amazon
- `karamelToken` — token DRM temporal por libro

**Flujo de autenticación:**
1. El usuario obtiene manualmente cookies de `read.amazon.com` (`ubid-main`, `at-main`, `x-main`, `session-id`) y un `deviceToken`
2. Estas se pasan al constructor y se serializan como headers HTTP
3. Amazon requiere fingerprinting TLS de Chrome — todas las requests van a través de un servidor local `tls-client-api` (Go, `https://github.com/bogdanfinn/tls-client-api`) que falsifica el TLS fingerprint

**Endpoints de Amazon consumidos:**
| Endpoint | Método | Propósito |
|---|---|---|
| `https://read.amazon.com/kindle-library/search` | GET | Lista la biblioteca con paginación |
| `https://read.amazon.com/service/web/register/getDeviceToken` | GET | Registra/obtiene info del dispositivo |
| `https://read.amazon.com/service/mobile/reader/startReading` | GET | Inicia lectura, obtiene karamelToken y metadataUrl |
| `<metadataUrl>` (S3 Amazon) | GET | Descarga metadatos JSONP del libro |
| `https://read.amazon.com/renderer/render` | GET | Obtiene manifest de contenido en formato TAR |

**Rate limiting:** máximo 3 requests/segundo para evitar detección.

## Patrones Go a seguir

### Functional options pattern para el constructor

```go
type Client struct { ... }

type Option func(*Client)

func WithThrottle(enabled bool) Option { ... }
func WithBaseURL(url string) Option { ... }

func NewClient(cookies Cookies, deviceToken string, opts ...Option) (*Client, error) { ... }
```

### Errores con tipos descriptivos

```go
type APIError struct {
    StatusCode int
    Message    string
}
func (e *APIError) Error() string { ... }
```

### Context en métodos públicos

Todos los métodos que hacen I/O deben aceptar `context.Context` como primer parámetro:

```go
func (c *Client) Init(ctx context.Context) error
func (c *Client) GetBookDetails(ctx context.Context, asin string) (*BookDetails, error)
```

### Separación interna vs. pública

- `internal/` para lógica no exportable (proxy TLS, parsing JSONP, etc.)
- Exportar solo lo necesario: `Client`, tipos de dominio, `Option`, errores públicos

## Tipos de dominio principales (referencia de `kindle-api/src/types.ts`)

- `Book` — datos básicos (ASIN, título, autores, URL de portada)
- `BookDetails` — extiende Book con metadatos completos (publisher, progreso de lectura, posición)
- `Cookies` — struct con los cuatro campos requeridos (`UbidMain`, `AtMain`, `XMain`, `SessionID`)
- `DeviceInfo` — info del dispositivo registrado
- `BooksQueryOptions` — parámetros de consulta (sort, origen, paginación, tamaño)
- `SortType`, `OriginType` — string types para ordenamiento y filtros

## Proxy TLS

La comunicación con Amazon requiere pasar por un servidor local que usa `tls-client-api`. El cliente Go envía un `POST /api/forward` con:

```json
{
  "tlsClientIdentifier": "chrome_112",
  "requestUrl": "https://read.amazon.com/...",
  "requestMethod": "GET",
  "requestHeaders": { ... },
  "withDefaultCookieJar": false
}
```

La respuesta contiene `body` (string), `status` (int) y `cookies`. Esta lógica va en `internal/tlsproxy/`.

## Convenciones

- Módulo: `github.com/rpero/kindle-go` (o el path que defina el usuario)
- Go 1.21+ para usar `log/slog` y generics donde aplique
- Tests con `testing` estándar + `testify` para assertions
- Sin dependencias HTTP externas: usar `net/http` estándar para el proxy TLS local
