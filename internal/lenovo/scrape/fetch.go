package scrape

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	httpTimeout      = 30 * time.Second
	maxBodyBytes     = 5 << 20
	errSnippetLen    = 200
	maxFetchAttempts = 3
	initialBackoff   = 300 * time.Millisecond
	maxBackoff       = 15 * time.Second
	max429Wait       = 120 * time.Second
)

var defaultClient = &http.Client{Timeout: httpTimeout}

// UserAgent identifica o cliente de forma transparente.
const UserAgent = "ecommerce-crawler-api/lenovo-api/1.0 (+https://webscraper.io/test-sites; study)"

func snippetFromBody(body []byte) string {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > errSnippetLen {
		snippet = snippet[:errSnippetLen] + "…"
	}
	return snippet
}

func parseRetryAfterSeconds(h http.Header) time.Duration {
	s := strings.TrimSpace(h.Get("Retry-After"))
	if s == "" {
		return 0
	}
	sec, err := strconv.Atoi(s)
	if err != nil || sec < 0 {
		return 0
	}
	d := time.Duration(sec) * time.Second
	if d > max429Wait {
		return max429Wait
	}
	return d
}

func backoffAfterFailure(attemptIndex int, hint time.Duration) time.Duration {
	if hint > 0 {
		return hint
	}
	d := initialBackoff * time.Duration(1<<uint(attemptIndex))
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func httpPageError(pageURL string, statusCode int, body []byte) error {
	return fmt.Errorf("GET %s: status %d, snippet: %s", pageURL, statusCode, snippetFromBody(body))
}

func doFetchOnce(ctx context.Context, client *http.Client, pageURL string) (*html.Node, bool, time.Duration, error) {
	if client == nil {
		client = defaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, false, 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, true, 0, fmt.Errorf("GET %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, true, 0, fmt.Errorf("read body %s: %w", pageURL, err)
	}
	if len(body) > maxBodyBytes {
		return nil, false, 0, fmt.Errorf("GET %s: body larger than %d bytes", pageURL, maxBodyBytes)
	}

	switch code := resp.StatusCode; {
	case code == http.StatusOK:
		root, parseErr := html.Parse(strings.NewReader(string(body)))
		if parseErr != nil {
			return nil, false, 0, fmt.Errorf("parse html %s: %w", pageURL, parseErr)
		}
		return root, false, 0, nil
	case code == http.StatusTooManyRequests:
		return nil, true, parseRetryAfterSeconds(resp.Header), httpPageError(pageURL, code, body)
	case code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout:
		return nil, true, 0, httpPageError(pageURL, code, body)
	case code >= 500:
		return nil, true, 0, httpPageError(pageURL, code, body)
	default:
		return nil, false, 0, httpPageError(pageURL, code, body)
	}
}

// FetchHTML baixa e parseia HTML com retries e backoff (alinhado ao cmd/books-crawler).
func FetchHTML(ctx context.Context, client *http.Client, pageURL string) (*html.Node, error) {
	var lastErr error
	var lastHint time.Duration
	for attempt := 0; attempt < maxFetchAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			wait := backoffAfterFailure(attempt-1, lastHint)
			lastHint = 0
			if err := sleepWithContext(ctx, wait); err != nil {
				return nil, err
			}
		}
		doc, retry, hint, err := doFetchOnce(ctx, client, pageURL)
		if err == nil {
			return doc, nil
		}
		lastErr = err
		lastHint = hint
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxFetchAttempts, lastErr)
}
