package tlsproxy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodrigopero/kindle-api-go/internal/tlsproxy"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *tlsproxy.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := tlsproxy.New(srv.URL, "test-key", 5*time.Second)
	return srv, client
}

func TestForward_Success(t *testing.T) {
	want := tlsproxy.ResponseData{
		Status: 200,
		Target: "https://read.amazon.com/test",
		Body:   `{"ok":true}`,
	}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/forward", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload tlsproxy.RequestPayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "https://read.amazon.com/test", payload.RequestURL)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	})

	payload := &tlsproxy.RequestPayload{
		RequestURL:    "https://read.amazon.com/test",
		RequestMethod: tlsproxy.MethodGET,
	}

	got, err := client.Forward(context.Background(), payload)
	require.NoError(t, err)
	assert.Equal(t, want.Status, got.Status)
	assert.Equal(t, want.Body, got.Body)
}

func TestForward_ServerUnavailable(t *testing.T) {
	client := tlsproxy.New("http://localhost:19999", "key", 500*time.Millisecond)

	_, err := client.Forward(context.Background(), &tlsproxy.RequestPayload{
		RequestURL:    "https://example.com",
		RequestMethod: tlsproxy.MethodGET,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tlsproxy: sending request to proxy")
}

func TestForward_ProxyNon200(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	_, err := client.Forward(context.Background(), &tlsproxy.RequestPayload{
		RequestURL:    "https://example.com",
		RequestMethod: tlsproxy.MethodGET,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestForward_ContextCancelled(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(tlsproxy.ResponseData{Status: 200})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Forward(ctx, &tlsproxy.RequestPayload{
		RequestURL:    "https://example.com",
		RequestMethod: tlsproxy.MethodGET,
	})

	require.Error(t, err)
}

func TestForward_InvalidJSON(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})

	_, err := client.Forward(context.Background(), &tlsproxy.RequestPayload{
		RequestURL:    "https://example.com",
		RequestMethod: tlsproxy.MethodGET,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tlsproxy: decoding proxy response")
}
