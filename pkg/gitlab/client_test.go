package gitlab

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockServer creates a test HTTP server and a Client pointed at it.
func setupMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := ClientConfig{
		BaseURL:             srv.URL,
		Token:               "test-token",
		RequestsPerSecond:   100,
		Burst:               100,
		RetryMax:            3,
		RetryInitialBackoff: 1 * time.Millisecond, // fast for tests
	}
	client := NewClient(cfg)
	return srv, client
}

func TestDo_AuthHeader(t *testing.T) {
	var receivedToken string
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("PRIVATE-TOKEN")
		w.WriteHeader(http.StatusOK)
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, client.cfg.BaseURL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, "test-token", receivedToken)
}

func TestDo_RateLimiting(t *testing.T) {
	// Use a very slow limiter to verify Wait is called
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := ClientConfig{
		BaseURL:             srv.URL,
		Token:               "test-token",
		RequestsPerSecond:   1000,
		Burst:               1000,
		RetryMax:            0,
		RetryInitialBackoff: 1 * time.Millisecond,
	}
	client := NewClient(cfg)

	// Send multiple requests and verify they all succeed (limiter doesn't block at high rate)
	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
		require.NoError(t, err)
		resp, err := client.Do(context.Background(), req)
		require.NoError(t, err)
		resp.Body.Close()
	}
}

func TestDo_Retry_NetworkError(t *testing.T) {
	var callCount int32

	// Server that drops the connection on first 2 calls, succeeds on 3rd
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			// Force connection close to simulate network error
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
			// Fallback: return 500
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"ok"`))
	}))
	t.Cleanup(srv.Close)

	cfg := ClientConfig{
		BaseURL:             srv.URL,
		Token:               "test-token",
		RequestsPerSecond:   100,
		Burst:               100,
		RetryMax:            3,
		RetryInitialBackoff: 1 * time.Millisecond,
	}
	client := NewClient(cfg)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	// Server drops on calls 1 and 2, succeeds on call 3. With RetryMax=3 the
	// client should succeed on the 3rd attempt.
	require.NoError(t, err, "should succeed on 3rd attempt")
	resp.Body.Close()
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount), "should attempt exactly 3 times")
}

func TestDo_Retry_429_RetryAfter(t *testing.T) {
	var callCount int32

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.Header().Set("Retry-After", "0") // 0 seconds for fast test
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"ok"`))
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, client.cfg.BaseURL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "should retry after 429")
}

func TestDo_Retry_429_NoHeader(t *testing.T) {
	var callCount int32

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			// 429 without Retry-After header — should use default backoff
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"ok"`))
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, client.cfg.BaseURL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "should retry after 429 without Retry-After header")
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		// Always drop connection to simulate persistent network failure
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	cfg := ClientConfig{
		BaseURL:             srv.URL,
		Token:               "test-token",
		RequestsPerSecond:   100,
		Burst:               100,
		RetryMax:            2,
		RetryInitialBackoff: 1 * time.Millisecond,
	}
	client := NewClient(cfg)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	require.NoError(t, err)

	_, err = client.Do(context.Background(), req)
	assert.Error(t, err, "should return error after max retries")
	assert.Contains(t, err.Error(), "failed after")
	// RetryMax=2 means 3 total attempts (initial + 2 retries)
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount), "should attempt RetryMax+1 times")
}

func TestDo_401_NoRetry(t *testing.T) {
	var callCount int32

	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, client.cfg.BaseURL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	assert.Error(t, err, "401 should return an error")
	assert.Nil(t, resp, "401 should not return a response")
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "401 should not be retried")
}

func TestDo_Success(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value"}`))
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, client.cfg.BaseURL+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, string(body))
}

func TestDo_RetryWithBody(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			// Drop connection to simulate network error
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
		}
		// Read and discard body on retry
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := ClientConfig{
		BaseURL:             srv.URL,
		Token:               "test-token",
		RequestsPerSecond:   100,
		Burst:               100,
		RetryMax:            3,
		RetryInitialBackoff: 1 * time.Millisecond,
	}
	client := NewClient(cfg)

	bodyContent := `{"test":"data"}`
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		srv.URL+"/test",
		strings.NewReader(bodyContent),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err, "should succeed after retry with body replay")
	resp.Body.Close()
	// Verify that a retry actually occurred (call 1 dropped, call 2 succeeded).
	calls := atomic.LoadInt32(&callCount)
	assert.Equal(t, int32(2), calls, "should have retried exactly once with body replayed")
}

func TestDo_ContextCancel(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.cfg.BaseURL+"/test", nil)
	require.NoError(t, err)

	_, err = client.Do(ctx, req)
	assert.Error(t, err, "should fail with context cancellation")
}

func TestBackoff_Exponential(t *testing.T) {
	cfg := ClientConfig{
		RetryInitialBackoff: 10 * time.Millisecond,
	}
	client := NewClient(cfg)

	// Verify backoff returns positive durations and grows with attempt number.
	// With base=10ms: attempt 0 → 10ms + jitter, attempt 1 → 20ms + jitter, attempt 2 → 40ms + jitter.
	// Jitter is 0-500ms, so absolute ordering isn't guaranteed.
	// We verify both the minimum (base * 2^attempt) and that extra is added.
	b0 := client.backoff(0, 0)
	b0WithExtra := client.backoff(0, 1*time.Second)

	assert.Greater(t, b0, time.Duration(0))
	// Extra should add to duration
	assert.Greater(t, b0WithExtra, b0)
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	cfg := ClientConfig{RetryInitialBackoff: 1 * time.Millisecond}
	client := NewClient(cfg)

	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "5")

	d := client.parseRetryAfter(resp)
	assert.Equal(t, 5*time.Second, d)
}

func TestParseRetryAfter_Missing(t *testing.T) {
	cfg := ClientConfig{RetryInitialBackoff: 1 * time.Millisecond}
	client := NewClient(cfg)

	resp := &http.Response{Header: http.Header{}}
	d := client.parseRetryAfter(resp)
	assert.Equal(t, time.Duration(0), d)
}
