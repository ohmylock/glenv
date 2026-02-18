package gitlab

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// ClientConfig holds configuration for the GitLab HTTP client.
type ClientConfig struct {
	BaseURL             string
	Token               string
	RequestsPerSecond   float64
	Burst               int
	RetryMax            int
	RetryInitialBackoff time.Duration
	HTTPClient          *http.Client
}

// Client is a rate-limited, retry-aware HTTP client for the GitLab API.
type Client struct {
	cfg     ClientConfig
	limiter *rate.Limiter
	http    *http.Client
}

// NewClient creates a new Client with the given configuration.
// Default values are applied for zero-value fields.
func NewClient(cfg ClientConfig) *Client {
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = 10
	}
	if cfg.Burst <= 0 {
		cfg.Burst = max(1, int(cfg.RequestsPerSecond))
	}
	if cfg.RetryMax <= 0 {
		cfg.RetryMax = 3
	}
	if cfg.RetryInitialBackoff <= 0 {
		cfg.RetryInitialBackoff = 1 * time.Second
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		cfg:     cfg,
		limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst),
		http:    httpClient,
	}
}

// Do executes an HTTP request with rate limiting, retry, and backoff.
// The PRIVATE-TOKEN header is injected automatically.
// 401 responses are returned immediately without retry.
// 429 responses are retried after honoring the Retry-After header.
// Network errors are retried up to RetryMax times with exponential backoff.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Buffer the request body for replay on retry.
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("gitlab: read request body: %w", err)
		}
		req.Body.Close()
	}

	req.Header.Set("PRIVATE-TOKEN", c.cfg.Token)

	var lastErr error
	for attempt := 0; attempt <= c.cfg.RetryMax; attempt++ {
		// Wait for the rate limiter.
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("gitlab: rate limiter: %w", err)
		}

		// Restore the body for this attempt.
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.cfg.RetryMax {
				sleep := c.backoff(attempt, 0)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(sleep):
				}
			}
			continue
		}

		// 401: do not retry.
		if resp.StatusCode == http.StatusUnauthorized {
			return resp, nil
		}

		// 429: respect Retry-After, then retry.
		if resp.StatusCode == http.StatusTooManyRequests {
			extra := c.parseRetryAfter(resp)
			resp.Body.Close()
			if attempt < c.cfg.RetryMax {
				sleep := c.backoff(attempt, extra)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(sleep):
				}
				continue
			}
			return nil, fmt.Errorf("gitlab: rate limited after %d attempts", attempt+1)
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("gitlab: request failed after %d attempts: %w", c.cfg.RetryMax+1, lastErr)
	}
	return nil, fmt.Errorf("gitlab: request failed after %d attempts", c.cfg.RetryMax+1)
}

// backoff calculates the sleep duration for a retry attempt.
// base * 2^attempt + jitter(0..500ms) + extra.
func (c *Client) backoff(attempt int, extra time.Duration) time.Duration {
	base := c.cfg.RetryInitialBackoff
	exp := time.Duration(1 << uint(attempt))
	jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
	return base*exp + jitter + extra
}

// parseRetryAfter reads the Retry-After header and returns the duration to wait.
// Returns 0 if the header is absent or unparseable.
func (c *Client) parseRetryAfter(resp *http.Response) time.Duration {
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(header, 64)
	if err != nil {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}
