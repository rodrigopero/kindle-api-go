package kindle

import "time"

// Option es una función que configura un Client durante su construcción.
type Option func(*Client)

// WithThrottle habilita o deshabilita el rate limiting (default: true).
// Con rate limiting activo se permiten máximo 3 requests por segundo.
func WithThrottle(enabled bool) Option {
	return func(c *Client) {
		c.throttle = enabled
	}
}

// WithBaseURL sobreescribe la URL base de la API de Amazon (default: "https://read.amazon.com").
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithClientVersion sobreescribe la versión del cliente Kindle (default: "20000100").
func WithClientVersion(v string) Option {
	return func(c *Client) {
		c.clientVersion = v
	}
}

// WithTLSProxyTimeout sobreescribe el timeout de las llamadas al proxy TLS (default: 30s).
func WithTLSProxyTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.proxyTimeout = d
	}
}
