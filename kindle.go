package kindle

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/rodrigopero/kindle-api-go/internal/tlsproxy"
)

const (
	defaultBaseURL       = "https://read.amazon.com"
	defaultClientVersion = "20000100"
	defaultQuerySize     = 50
	defaultProxyTimeout  = 30 * time.Second
	userAgent            = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36"
	tlsClientIdentifier  = "chrome_112"
)

// proxyForwarder permite inyectar un mock en los tests sin exponer la interfaz.
type proxyForwarder interface {
	Forward(ctx context.Context, payload *tlsproxy.RequestPayload) (*tlsproxy.ResponseData, error)
}

// Client es el cliente principal para la API privada de Kindle.
// Se debe crear con NewClient y llamar a Init antes de usar otros métodos.
type Client struct {
	cookies       Cookies
	deviceToken   string
	baseURL       string
	clientVersion string
	throttle      bool
	proxyTimeout  time.Duration

	proxy proxyForwarder

	sessionID    string
	adpSessionID string
	karamelToken *KaramelToken

	limiter *rate.Limiter

	// Books contiene todos los libros de la biblioteca tras llamar a Init.
	Books []Book
}

// NewClient construye un nuevo Client con las cookies y deviceToken proporcionados.
// tlsServerURL es la URL del servidor tls-client-api (ej: "http://localhost:8080").
func NewClient(
	cookies Cookies,
	deviceToken string,
	tlsServerURL string,
	tlsServerAPIKey string,
	opts ...Option,
) (*Client, error) {
	if cookies.UbidMain == "" || cookies.AtMain == "" ||
		cookies.SessionID == "" || cookies.XMain == "" {
		return nil, fmt.Errorf("kindle: all four cookies are required (UbidMain, AtMain, SessionID, XMain)")
	}
	if deviceToken == "" {
		return nil, fmt.Errorf("kindle: deviceToken is required")
	}
	if tlsServerURL == "" {
		return nil, fmt.Errorf("kindle: tlsServerURL is required")
	}
	if tlsServerAPIKey == "" {
		return nil, fmt.Errorf("kindle: tlsServerAPIKey is required")
	}

	c := &Client{
		cookies:       cookies,
		deviceToken:   deviceToken,
		baseURL:       defaultBaseURL,
		clientVersion: defaultClientVersion,
		throttle:      true,
		proxyTimeout:  defaultProxyTimeout,
		sessionID:     cookies.SessionID,
		// Un token cada 333ms = máximo 3 requests/segundo, sin burst acumulado
		limiter: rate.NewLimiter(rate.Every(time.Second/3), 1),
	}

	for _, opt := range opts {
		opt(c)
	}

	c.proxy = tlsproxy.New(tlsServerURL, tlsServerAPIKey, c.proxyTimeout)

	return c, nil
}

// Init carga la biblioteca completa y registra el dispositivo.
// Debe llamarse antes de usar GetBookDetails o GetBookContentManifest.
func (c *Client) Init(ctx context.Context) error {
	if err := c.getAllBooks(ctx, BooksQueryOptions{}); err != nil {
		return fmt.Errorf("kindle: loading library: %w", err)
	}

	if _, err := c.UpdateDeviceInfo(ctx); err != nil {
		return fmt.Errorf("kindle: updating device info: %w", err)
	}

	return nil
}

// UpdateDeviceInfo registra el dispositivo y obtiene su información.
// Actualiza el adpSessionID interno usado en requests subsiguientes.
func (c *Client) UpdateDeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	url := fmt.Sprintf(
		"%s/service/web/register/getDeviceToken?serialNumber=%s&deviceType=%s",
		c.baseURL,
		c.deviceToken,
		c.deviceToken,
	)

	res, err := c.request(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("kindle: getDeviceToken: %w", err)
	}

	var info DeviceInfo
	if err := json.Unmarshal([]byte(res.Body), &info); err != nil {
		return nil, fmt.Errorf("kindle: parsing device info: %w", err)
	}

	c.adpSessionID = info.DeviceSessionToken
	return &info, nil
}

// GetBookDetails obtiene los metadatos completos de un libro por su ASIN.
// Realiza dos requests: startReading (karamelToken + metadataUrl) y la URL de metadatos S3 (JSONP).
func (c *Client) GetBookDetails(ctx context.Context, asin string) (*BookDetails, error) {
	if asin == "" {
		return nil, fmt.Errorf("kindle: asin is required")
	}

	var baseBook *Book
	for i := range c.Books {
		if c.Books[i].ASIN == asin {
			baseBook = &c.Books[i]
			break
		}
	}

	startURL := fmt.Sprintf(
		"%s/service/mobile/reader/startReading?asin=%s&clientVersion=%s",
		c.baseURL,
		asin,
		c.clientVersion,
	)

	res0, err := c.request(ctx, startURL, nil)
	if err != nil {
		return nil, fmt.Errorf("kindle: startReading for %s: %w", asin, err)
	}

	var info startReadingResponse
	if err := json.Unmarshal([]byte(res0.Body), &info); err != nil {
		return nil, fmt.Errorf("kindle: parsing startReading response: %w", err)
	}
	c.karamelToken = &info.KaramelToken

	res1, err := c.request(ctx, info.MetadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("kindle: fetching metadata for %s: %w", asin, err)
	}

	meta, err := parseJSONPResponse[bookMetadataResponse](res1.Body)
	if err != nil {
		return nil, fmt.Errorf("kindle: parsing metadata JSONP for %s: %w", asin, err)
	}

	roughDecimal := float64(meta.StartPosition+info.LastPageReadData.Position) / float64(meta.EndPosition)
	percentageRead := math.Round(roughDecimal*1000) / 10

	details := &BookDetails{
		BookLightDetails: BookLightDetails{
			BookType:      bookTypeFromInfo(info.IsOwned, info.IsSample),
			FormatVersion: info.FormatVersion,
			MetadataURL:   info.MetadataURL,
			SRL:           info.SRL,
			Progress: ReadingProgress{
				ReportedOnDevice: info.LastPageReadData.DeviceName,
				Position:         info.LastPageReadData.Position,
				SyncDate:         time.UnixMilli(info.LastPageReadData.SyncTime),
			},
		},
		ReleaseDate:    meta.ReleaseDate,
		StartPosition:  meta.StartPosition,
		EndPosition:    meta.EndPosition,
		Publisher:      meta.Publisher,
		PercentageRead: percentageRead,
	}

	if baseBook != nil {
		details.Book = *baseBook
		details.LargeCoverURL = toLargeImage(baseBook.ProductURL)
	} else {
		details.Book = Book{
			ASIN:    asin,
			Title:   meta.Title,
			Authors: meta.AuthorList,
		}
	}

	return details, nil
}

// GetBookContentManifest obtiene el manifest de contenido del libro en formato TAR.
func (c *Client) GetBookContentManifest(ctx context.Context, asin string) (string, error) {
	if asin == "" {
		return "", fmt.Errorf("kindle: asin is required")
	}

	params := map[string]string{
		"version":          "3.0",
		"asin":             asin,
		"contentType":      "FullBook",
		"revision":         "da38557c",
		"fontFamily":       "Bookerly",
		"fontSize":         "8.91",
		"lineHeight":       "1.4",
		"dpi":              "160",
		"height":           "222",
		"width":            "1384",
		"marginBottom":     "0",
		"marginLeft":       "9",
		"marginRight":      "9",
		"marginTop":        "0",
		"maxNumberColumns": "2",
		"theme":            "default",
		"locationMap":      "true",
		"packageType":      "TAR",
		"encryptionVersion": "NONE",
		"numPage":          "6",
		"skipPageCount":    "0",
		"startingPosition": "162515",
		"bundleImages":     "false",
	}

	if c.karamelToken != nil {
		params["token"] = c.karamelToken.Token
	}

	u := buildURL(fmt.Sprintf("%s/renderer/render", c.baseURL), params)

	res, err := c.request(ctx, u, nil)
	if err != nil {
		return "", fmt.Errorf("kindle: getBookContentManifest for %s: %w", asin, err)
	}

	if res.Body == "" {
		return "", &APIError{
			StatusCode: res.Status,
			Message:    "empty body: you likely need to refresh your cookies",
		}
	}

	return res.Body, nil
}

// getAllBooks carga todos los libros de la biblioteca, paginando si es necesario.
func (c *Client) getAllBooks(ctx context.Context, opts BooksQueryOptions) error {
	params := BooksQueryOptions{
		SortType:  SortTypeRecency,
		QuerySize: defaultQuerySize,
	}
	if opts.SortType != "" {
		params.SortType = opts.SortType
	}
	if opts.OriginType != "" {
		params.OriginType = opts.OriginType
	}
	if opts.QuerySize > 0 {
		params.QuerySize = opts.QuerySize
	}
	if opts.FetchAllPages {
		params.FetchAllPages = true
	}

	var allBooks []Book

	for {
		books, paginationToken, sessionID, err := c.getBooks(ctx, params)
		if err != nil {
			return err
		}

		if sessionID != "" {
			c.sessionID = sessionID
		}
		allBooks = append(allBooks, books...)

		if !params.FetchAllPages || paginationToken == "" ||
			(params.QuerySize > 0 && len(allBooks) >= params.QuerySize) {
			break
		}

		params.PaginationToken = paginationToken
	}

	c.Books = allBooks
	return nil
}

// getBooks realiza una sola request paginada a la API de la biblioteca.
func (c *Client) getBooks(ctx context.Context, opts BooksQueryOptions) ([]Book, string, string, error) {
	baseURL := fmt.Sprintf("%s/kindle-library/search?query=&libraryType=BOOKS", c.baseURL)

	queryParams := map[string]string{
		"sortType":  string(opts.SortType),
		"querySize": fmt.Sprintf("%d", opts.QuerySize),
	}
	if opts.OriginType != "" {
		queryParams["originType"] = string(opts.OriginType)
	}
	if opts.PaginationToken != "" {
		queryParams["paginationToken"] = opts.PaginationToken
	}

	u := buildURL(baseURL, queryParams)

	res, err := c.request(ctx, u, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("kindle: fetching books page: %w", err)
	}

	if res.Body == "" {
		return nil, "", "", &APIError{
			StatusCode: res.Status,
			Message:    "empty body: you likely need to refresh your cookies",
		}
	}

	var body booksListResponse
	if err := json.Unmarshal([]byte(res.Body), &body); err != nil {
		return nil, "", "", fmt.Errorf("kindle: parsing books response: %w", err)
	}

	books := make([]Book, 0, len(body.ItemsList))
	for _, item := range body.ItemsList {
		item.Book.Authors = normalizeAuthors(item.Book.Authors)
		books = append(books, item.Book)
	}

	sessionID := res.Cookies["session-id"]

	return books, body.PaginationToken, sessionID, nil
}

// request envía una request a Amazon a través del proxy TLS local.
// Aplica rate limiting si está habilitado y agrega los headers de sesión necesarios.
func (c *Client) request(ctx context.Context, url string, payload *tlsproxy.RequestPayload) (*tlsproxy.ResponseData, error) {
	if c.throttle {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("kindle: rate limiter: %w", err)
		}
	}

	headers := map[string]string{
		"Cookie":          serializeCookies(c.cookies),
		"Accept-Language": "en-US,en;q=0.9,ko-KR;q=0.8,ko;q=0.7",
		"User-Agent":      userAgent,
	}

	if c.sessionID != "" {
		headers["x-amzn-sessionid"] = c.sessionID
	}

	if c.adpSessionID != "" {
		headers["x-adp-session-token"] = c.adpSessionID
	}

	if payload != nil {
		for k, v := range payload.Headers {
			headers[k] = v
		}
	}

	withDebug := true
	tlsPayload := &tlsproxy.RequestPayload{
		TLSClientIdentifier: tlsClientIdentifier,
		RequestURL:          url,
		RequestMethod:       tlsproxy.MethodGET,
		WithDebug:           &withDebug,
		Headers:             headers,
	}

	if payload != nil {
		tlsPayload.RequestMethod = payload.RequestMethod
		if payload.RequestBody != "" {
			tlsPayload.RequestBody = payload.RequestBody
		}
	}

	res, err := c.proxy.Forward(ctx, tlsPayload)
	if err != nil {
		return nil, err
	}

	if res.Status >= 400 {
		return nil, &APIError{
			StatusCode: res.Status,
			Message:    res.Body,
		}
	}

	return res, nil
}

// --- Helpers ---

func bookTypeFromInfo(isOwned, isSample bool) BookType {
	switch {
	case isSample:
		return BookTypeSample
	case isOwned:
		return BookTypeOwned
	default:
		return BookTypeUnknown
	}
}

// normalizeAuthors convierte "LastName, FirstName:LastName2, FirstName2" → ["FirstName LastName", ...].
func normalizeAuthors(rawAuthors []string) []string {
	if len(rawAuthors) == 0 {
		return nil
	}

	rawAuthor := rawAuthors[0]
	parts := strings.Split(rawAuthor, ":")

	seen := make(map[string]struct{})
	var result []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		elems := strings.Split(part, ",")
		for i, e := range elems {
			elems[i] = strings.TrimSpace(e)
		}
		for i, j := 0, len(elems)-1; i < j; i, j = i+1, j-1 {
			elems[i], elems[j] = elems[j], elems[i]
		}
		normalized := strings.Join(elems, " ")

		if _, exists := seen[normalized]; !exists {
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}

	return result
}

var sizeRegexp = regexp.MustCompile(`\._SY\d+_\.`)

// toLargeImage elimina el sufijo de resolución de las URLs de portada de Amazon.
func toLargeImage(url string) string {
	return sizeRegexp.ReplaceAllString(url, ".")
}

// serializeCookies convierte el struct Cookies al header Cookie de HTTP.
func serializeCookies(c Cookies) string {
	return strings.Join([]string{
		"ubid-main=" + c.UbidMain,
		"at-main=" + c.AtMain,
		"session-id=" + c.SessionID,
		"x-main=" + c.XMain,
	}, "; ")
}

// DeserializeCookies parsea un string de cookies al struct Cookies.
// Útil para cargar cookies desde una variable de entorno.
func DeserializeCookies(raw string) (Cookies, error) {
	values := make(map[string]string)
	for _, part := range strings.Split(raw, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			values[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	c := Cookies{
		UbidMain:  values["ubid-main"],
		AtMain:    values["at-main"],
		SessionID: values["session-id"],
		XMain:     values["x-main"],
	}

	if c.UbidMain == "" || c.AtMain == "" || c.SessionID == "" || c.XMain == "" {
		return Cookies{}, fmt.Errorf("kindle: missing required cookies in string")
	}

	return c, nil
}

var jsonpRegexp = regexp.MustCompile(`\((\{.*\})\)`)

// parseJSONPResponse extrae el JSON del wrapper JSONP y lo deserializa en T.
func parseJSONPResponse[T any](body string) (*T, error) {
	matches := jsonpRegexp.FindStringSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("no JSONP wrapper found in response")
	}

	var result T
	if err := json.Unmarshal([]byte(matches[1]), &result); err != nil {
		return nil, fmt.Errorf("parsing JSONP content: %w", err)
	}

	return &result, nil
}

// buildURL agrega query parameters a una URL base manteniendo el orden de inserción.
func buildURL(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}

	var parts []string
	for k, v := range params {
		parts = append(parts, k+"="+v)
	}

	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}

	return base + sep + strings.Join(parts, "&")
}
