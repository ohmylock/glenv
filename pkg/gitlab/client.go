package gitlab

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
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
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
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
	// Clone to avoid mutating the caller's request (token must not leak via shared headers).
	req = req.Clone(ctx)

	// Buffer the request body for replay on retry.
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("gitlab: read request body: %w", err)
		}
		_ = req.Body.Close()
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

		// 401: do not retry — return a clear authentication error.
		if resp.StatusCode == http.StatusUnauthorized {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("gitlab: authentication failed (HTTP 401): verify your PRIVATE-TOKEN")
		}

		// 429: respect Retry-After, then retry.
		if resp.StatusCode == http.StatusTooManyRequests {
			extra := c.parseRetryAfter(resp)
			_ = resp.Body.Close()
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

		// 5xx: transient server errors, retry with backoff.
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("gitlab: server error %d", resp.StatusCode)
			if attempt < c.cfg.RetryMax {
				sleep := c.backoff(attempt, 0)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(sleep):
				}
				continue
			}
			return nil, fmt.Errorf("gitlab: server error %d after %d attempts", resp.StatusCode, attempt+1)
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("gitlab: request failed after %d attempts: %w", c.cfg.RetryMax+1, lastErr)
	}
	return nil, fmt.Errorf("gitlab: request failed after %d attempts", c.cfg.RetryMax+1)
}

// maxBackoff is the upper bound for any computed backoff duration.
const maxBackoff = 5 * time.Minute

// backoff calculates the sleep duration for a retry attempt.
// base * 2^attempt + jitter(0..500ms) + extra, capped at maxBackoff.
// attempt is capped at 30 to prevent integer overflow in the shift.
func (c *Client) backoff(attempt int, extra time.Duration) time.Duration {
	if attempt > 30 {
		attempt = 30
	}
	base := c.cfg.RetryInitialBackoff
	exp := time.Duration(1 << uint(attempt))
	// Guard against int64 overflow: if base alone exceeds maxBackoff, clamp early.
	if base > maxBackoff {
		base = maxBackoff
	}
	d := base*exp + extra
	if d > maxBackoff || d < 0 { // d < 0 catches any residual overflow
		d = maxBackoff
	}
	jitter := time.Duration(rand.Int64N(int64(500 * time.Millisecond)))
	result := d + jitter
	if result > maxBackoff {
		result = maxBackoff
	}
	return result
}

// parseRetryAfter reads the Retry-After header and returns the duration to wait.
// Returns 0 if the header is absent, unparseable, or negative.
func (c *Client) parseRetryAfter(resp *http.Response) time.Duration {
	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(header, 64)
	if err != nil || secs < 0 {
		return 0
	}
	d := time.Duration(secs * float64(time.Second))
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
