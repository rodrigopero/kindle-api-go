package tlsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RequestMethod enumera los métodos HTTP soportados por el proxy TLS.
type RequestMethod string

const (
	MethodGET    RequestMethod = "GET"
	MethodPOST   RequestMethod = "POST"
	MethodPATCH  RequestMethod = "PATCH"
	MethodPUT    RequestMethod = "PUT"
	MethodDELETE RequestMethod = "DELETE"
)

// RequestPayload es el cuerpo del POST a /api/forward del servidor tls-client-api.
// Los campos booleanos opcionales usan *bool para distinguir false de "no especificado".
type RequestPayload struct {
	TLSClientIdentifier         string              `json:"tlsClientIdentifier,omitempty"`
	RequestURL                  string              `json:"requestUrl"`
	RequestMethod               RequestMethod       `json:"requestMethod"`
	RequestBody                 string              `json:"requestBody,omitempty"`
	RequestCookies              []map[string]string `json:"requestCookies,omitempty"`
	FollowRedirects             *bool               `json:"followRedirects,omitempty"`
	InsecureSkipVerify          *bool               `json:"insecureSkipVerify,omitempty"`
	IsByteResponse              *bool               `json:"isByteResponse,omitempty"`
	WithoutCookieJar            *bool               `json:"withoutCookieJar,omitempty"`
	WithDebug                   *bool               `json:"withDebug,omitempty"`
	WithRandomTLSExtensionOrder *bool               `json:"withRandomTLSExtensionOrder,omitempty"`
	TimeoutSeconds              *int                `json:"timeoutSeconds,omitempty"`
	SessionID                   string              `json:"sessionId,omitempty"`
	ProxyURL                    string              `json:"proxyUrl,omitempty"`
	Headers                     map[string]string   `json:"headers,omitempty"`
	HeaderOrder                 []string            `json:"headerOrder,omitempty"`
}

// ResponseData es la respuesta del servidor tls-client-api.
type ResponseData struct {
	Status    int                 `json:"status"`
	Target    string              `json:"target"`
	Body      string              `json:"body"`
	Headers   map[string][]string `json:"headers"`
	Cookies   map[string]string   `json:"cookies"`
	SessionID string              `json:"sessionId,omitempty"`
}

// Client es el cliente para el servidor local tls-client-api.
type Client struct {
	serverURL  string
	apiKey     string
	httpClient *http.Client
}

// New crea un nuevo Client para el proxy TLS local.
func New(serverURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		serverURL: serverURL,
		apiKey:    apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Forward envía una request al servidor tls-client-api para que la reenvíe
// a Amazon con el fingerprint TLS correcto.
func (c *Client) Forward(ctx context.Context, payload *RequestPayload) (*ResponseData, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tlsproxy: marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.serverURL+"/api/forward",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("tlsproxy: creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tlsproxy: sending request to proxy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tlsproxy: proxy returned status %d: %s", resp.StatusCode, raw)
	}

	var result ResponseData
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tlsproxy: decoding proxy response: %w", err)
	}

	return &result, nil
}
