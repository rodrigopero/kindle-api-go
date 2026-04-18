package kindle

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodrigopero/kindle-api-go/internal/tlsproxy"
)

// mockProxy implementa proxyForwarder para tests.
type mockProxy struct {
	responses []*tlsproxy.ResponseData
	errors    []error
	calls     int
}

func (m *mockProxy) Forward(_ context.Context, _ *tlsproxy.RequestPayload) (*tlsproxy.ResponseData, error) {
	i := m.calls
	m.calls++
	if i < len(m.errors) && m.errors[i] != nil {
		return nil, m.errors[i]
	}
	if i < len(m.responses) {
		return m.responses[i], nil
	}
	return &tlsproxy.ResponseData{Status: 200, Body: "{}"}, nil
}

func validCookies() Cookies {
	return Cookies{UbidMain: "u", AtMain: "a", SessionID: "s", XMain: "x"}
}

func newTestClient(t *testing.T, proxy proxyForwarder) *Client {
	t.Helper()
	c := &Client{
		cookies:       validCookies(),
		deviceToken:   "token",
		baseURL:       defaultBaseURL,
		clientVersion: defaultClientVersion,
		throttle:      false,
		proxy:         proxy,
		sessionID:     "s",
		limiter:       nil,
	}
	return c
}

// --- Constructor ---

func TestNewClient_MissingCookies(t *testing.T) {
	_, err := NewClient(Cookies{}, "token", "http://localhost", "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cookies are required")
}

func TestNewClient_MissingDeviceToken(t *testing.T) {
	_, err := NewClient(validCookies(), "", "http://localhost", "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deviceToken")
}

func TestNewClient_MissingTLSServerURL(t *testing.T) {
	_, err := NewClient(validCookies(), "token", "", "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tlsServerURL")
}

func TestNewClient_MissingAPIKey(t *testing.T) {
	_, err := NewClient(validCookies(), "token", "http://localhost", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tlsServerAPIKey")
}

func TestNewClient_Options(t *testing.T) {
	c, err := NewClient(
		validCookies(), "token", "http://localhost", "key",
		WithThrottle(false),
		WithBaseURL("http://custom"),
		WithClientVersion("99"),
		WithTLSProxyTimeout(10*time.Second),
	)
	require.NoError(t, err)
	assert.False(t, c.throttle)
	assert.Equal(t, "http://custom", c.baseURL)
	assert.Equal(t, "99", c.clientVersion)
	assert.Equal(t, 10*time.Second, c.proxyTimeout)
}

// --- UpdateDeviceInfo ---

func TestUpdateDeviceInfo_Success(t *testing.T) {
	info := DeviceInfo{
		ClientHashID:       "hash",
		DeviceName:         "TestDevice",
		DeviceSessionToken: "adp-token",
		EID:                "eid",
	}
	body, _ := json.Marshal(info)

	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: string(body)},
	}}
	c := newTestClient(t, proxy)

	got, err := c.UpdateDeviceInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, info.DeviceName, got.DeviceName)
	assert.Equal(t, "adp-token", c.adpSessionID)
}

func TestUpdateDeviceInfo_ProxyError(t *testing.T) {
	proxy := &mockProxy{errors: []error{fmt.Errorf("network error")}}
	c := newTestClient(t, proxy)

	_, err := c.UpdateDeviceInfo(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getDeviceToken")
}

// --- Init ---

func TestInit_Success(t *testing.T) {
	booksResp := booksListResponse{
		ItemsList: []struct {
			Book
			PercentageRead float64 `json:"percentageRead,omitempty"`
		}{
			{Book: Book{ASIN: "B001", Title: "Book One", Authors: []string{"Doe, John"}}},
			{Book: Book{ASIN: "B002", Title: "Book Two", Authors: []string{"Smith, Jane"}}},
		},
	}
	booksBody, _ := json.Marshal(booksResp)

	deviceInfo := DeviceInfo{DeviceSessionToken: "tok"}
	deviceBody, _ := json.Marshal(deviceInfo)

	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: string(booksBody), Cookies: map[string]string{}},
		{Status: 200, Body: string(deviceBody)},
	}}
	c := newTestClient(t, proxy)

	err := c.Init(context.Background())
	require.NoError(t, err)
	assert.Len(t, c.Books, 2)
	assert.Equal(t, "tok", c.adpSessionID)
}

func TestInit_BooksError(t *testing.T) {
	proxy := &mockProxy{errors: []error{fmt.Errorf("timeout")}}
	c := newTestClient(t, proxy)

	err := c.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading library")
}

// --- GetBookDetails ---

func TestGetBookDetails_EmptyASIN(t *testing.T) {
	c := newTestClient(t, &mockProxy{})
	_, err := c.GetBookDetails(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "asin is required")
}

func TestGetBookDetails_Success(t *testing.T) {
	startResp := startReadingResponse{
		IsOwned:       true,
		FormatVersion: "v2",
		MetadataURL:   "https://example.com/meta",
		SRL:           100,
		KaramelToken:  KaramelToken{Token: "kt", ExpiresAt: 9999},
		LastPageReadData: lastPageReadData{
			DeviceName: "Kindle",
			Position:   500,
			SyncTime:   1700000000000,
		},
	}
	startBody, _ := json.Marshal(startResp)

	meta := bookMetadataResponse{
		ASIN:          "B001",
		Title:         "Test Book",
		Publisher:     "Pub",
		ReleaseDate:   "2023-01-01",
		StartPosition: 0,
		EndPosition:   1000,
		AuthorList:    []string{"Fischer, Travis"},
	}
	metaBody, _ := json.Marshal(meta)
	jsonpBody := fmt.Sprintf("callback(%s)", string(metaBody))

	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: string(startBody)},
		{Status: 200, Body: jsonpBody},
	}}
	c := newTestClient(t, proxy)
	c.Books = []Book{{ASIN: "B001", Title: "Test Book", ProductURL: "https://amazon.com/B001._SY98_.jpg"}}

	details, err := c.GetBookDetails(context.Background(), "B001")
	require.NoError(t, err)
	assert.Equal(t, "B001", details.ASIN)
	assert.Equal(t, BookTypeOwned, details.BookType)
	assert.Equal(t, "Pub", details.Publisher)
	assert.Equal(t, "kt", c.karamelToken.Token)
	// LargeCoverURL debe tener el sufijo eliminado
	assert.Equal(t, "https://amazon.com/B001.jpg", details.LargeCoverURL)
}

func TestGetBookDetails_JSONPParseError(t *testing.T) {
	startResp := startReadingResponse{MetadataURL: "https://example.com/meta"}
	startBody, _ := json.Marshal(startResp)

	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: string(startBody)},
		{Status: 200, Body: "not-jsonp"},
	}}
	c := newTestClient(t, proxy)

	_, err := c.GetBookDetails(context.Background(), "B001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSONP")
}

// --- getBooks ---

func TestGetBooks_EmptyBody(t *testing.T) {
	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: ""},
	}}
	c := newTestClient(t, proxy)

	_, _, _, err := c.getBooks(context.Background(), BooksQueryOptions{QuerySize: 10, SortType: SortTypeRecency})
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Contains(t, apiErr.Message, "refresh your cookies")
}

func TestGetBooks_SessionIDFromCookies(t *testing.T) {
	booksResp := booksListResponse{}
	body, _ := json.Marshal(booksResp)

	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: string(body), Cookies: map[string]string{"session-id": "new-session"}},
	}}
	c := newTestClient(t, proxy)

	_, _, sessionID, err := c.getBooks(context.Background(), BooksQueryOptions{QuerySize: 10, SortType: SortTypeRecency})
	require.NoError(t, err)
	assert.Equal(t, "new-session", sessionID)
}

// --- GetBookContentManifest ---

func TestGetBookContentManifest_EmptyASIN(t *testing.T) {
	c := newTestClient(t, &mockProxy{})
	_, err := c.GetBookContentManifest(context.Background(), "")
	require.Error(t, err)
}

func TestGetBookContentManifest_EmptyBody(t *testing.T) {
	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: ""},
	}}
	c := newTestClient(t, proxy)

	_, err := c.GetBookContentManifest(context.Background(), "B001")
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Contains(t, apiErr.Message, "refresh your cookies")
}

func TestGetBookContentManifest_Success(t *testing.T) {
	proxy := &mockProxy{responses: []*tlsproxy.ResponseData{
		{Status: 200, Body: "TAR_CONTENT"},
	}}
	c := newTestClient(t, proxy)

	result, err := c.GetBookContentManifest(context.Background(), "B001")
	require.NoError(t, err)
	assert.Equal(t, "TAR_CONTENT", result)
}

// --- Helpers ---

func TestNormalizeAuthors_Single(t *testing.T) {
	got := normalizeAuthors([]string{"Fischer, Travis"})
	assert.Equal(t, []string{"Travis Fischer"}, got)
}

func TestNormalizeAuthors_Multiple(t *testing.T) {
	got := normalizeAuthors([]string{"Knuth, Donald:Dijkstra, Edsger"})
	assert.Equal(t, []string{"Donald Knuth", "Edsger Dijkstra"}, got)
}

func TestNormalizeAuthors_Deduplication(t *testing.T) {
	got := normalizeAuthors([]string{"Smith, John:Smith, John"})
	assert.Equal(t, []string{"John Smith"}, got)
}

func TestNormalizeAuthors_Empty(t *testing.T) {
	assert.Nil(t, normalizeAuthors(nil))
	assert.Nil(t, normalizeAuthors([]string{}))
}

func TestToLargeImage(t *testing.T) {
	url := "https://amazon.com/cover._SY98_.jpg"
	assert.Equal(t, "https://amazon.com/cover.jpg", toLargeImage(url))
}

func TestToLargeImage_NoSuffix(t *testing.T) {
	url := "https://amazon.com/cover.jpg"
	assert.Equal(t, url, toLargeImage(url))
}

func TestSerializeCookies(t *testing.T) {
	c := Cookies{UbidMain: "u", AtMain: "a", SessionID: "s", XMain: "x"}
	got := serializeCookies(c)
	assert.Equal(t, "ubid-main=u; at-main=a; session-id=s; x-main=x", got)
}

func TestDeserializeCookies_Success(t *testing.T) {
	raw := "ubid-main=u; at-main=a; session-id=s; x-main=x"
	c, err := DeserializeCookies(raw)
	require.NoError(t, err)
	assert.Equal(t, "u", c.UbidMain)
	assert.Equal(t, "a", c.AtMain)
	assert.Equal(t, "s", c.SessionID)
	assert.Equal(t, "x", c.XMain)
}

func TestDeserializeCookies_Missing(t *testing.T) {
	_, err := DeserializeCookies("ubid-main=u; at-main=a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required cookies")
}

func TestParseJSONPResponse_Success(t *testing.T) {
	type payload struct {
		Key string `json:"key"`
	}
	got, err := parseJSONPResponse[payload](`callback({"key":"val"})`)
	require.NoError(t, err)
	assert.Equal(t, "val", got.Key)
}

func TestParseJSONPResponse_NoWrapper(t *testing.T) {
	_, err := parseJSONPResponse[map[string]any](`{"key":"val"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no JSONP wrapper")
}

func TestBookTypeFromInfo(t *testing.T) {
	assert.Equal(t, BookTypeSample, bookTypeFromInfo(true, true))
	assert.Equal(t, BookTypeOwned, bookTypeFromInfo(true, false))
	assert.Equal(t, BookTypeUnknown, bookTypeFromInfo(false, false))
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 403, Message: "forbidden"}
	assert.Equal(t, "kindle API error 403: forbidden", err.Error())
}
