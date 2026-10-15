package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	ebirdMaxAttempts  = 5
	ebirdMaxWait      = 5 * time.Minute
	ebirdFallbackWait = 2 * time.Second
	ebirdMaxBodyBytes = 64 << 20
)

// ebirdSleep waits for d or until ctx is done; replaced in tests.
var ebirdSleep = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ebirdGet requests endpoint with the eBird token and returns the body. On 429 or 503 it waits for
// the time given by Retry-After (or an exponential fallback) and tries again.
func ebirdGet(ctx context.Context, client *http.Client, apiKey, endpoint string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("create eBird request: %w", err)
		}
		req.Header.Set("X-eBirdApiToken", apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("eBird request: %w", err)
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
		if !retryable {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("eBird request: %s", resp.Status)
			}
			data, err := io.ReadAll(io.LimitReader(resp.Body, ebirdMaxBodyBytes))
			if err != nil {
				return nil, fmt.Errorf("read eBird response: %w", err)
			}
			return data, nil
		}

		status, header := resp.Status, resp.Header.Get("Retry-After")
		resp.Body.Close()
		wait, source := ebirdRetryDelay(header, attempt, time.Now())
		if attempt >= ebirdMaxAttempts || wait > ebirdMaxWait {
			return nil, fmt.Errorf("eBird request: %s after %d attempt(s); server asked to wait %s", status, attempt, wait)
		}
		slog.Debug("eBird rate limited; retrying", "status", status, "attempt", attempt,
			"delay", wait.String(), "delaySource", source)
		if err := ebirdSleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// ebirdRetryDelay reads Retry-After (seconds or HTTP date), falling back to exponential backoff.
func ebirdRetryDelay(header string, attempt int, now time.Time) (time.Duration, string) {
	header = strings.TrimSpace(header)
	if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, "Retry-After seconds"
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := t.Sub(now); d > 0 {
			return d, "Retry-After date"
		}
		return 0, "Retry-After date"
	}
	return ebirdFallbackWait << (attempt - 1), "exponential backoff"
}
