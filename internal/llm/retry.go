package llm

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// HTTPStatusError carries the raw HTTP status
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return "llm: http " + strconv.Itoa(e.StatusCode) + ": " + e.Body
}

// isRetryable reports whether the HTTP status code warrants a retry.
func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode >= 500
}

var backoffDurations = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}

// WithRetry calls fn up to maxRetries+1 times, backing off on retryable errors, returns (response, httpErr, err)
func WithRetry(ctx context.Context, maxRetries int, fn func() (Response, *HTTPStatusError, error)) (Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, httpErr, err := fn()
		if err == nil && httpErr == nil {
			return resp, nil
		}

		if httpErr != nil {
			if !isRetryable(httpErr.StatusCode) {
				return Response{}, httpErr
			}
			lastErr = httpErr
		} else {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return Response{}, err
			}
			lastErr = err
		}

		if attempt == maxRetries {
			break
		}

		idx := attempt
		if idx >= len(backoffDurations) {
			idx = len(backoffDurations) - 1
		}
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(backoffDurations[idx]):
		}
	}
	return Response{}, lastErr
}
