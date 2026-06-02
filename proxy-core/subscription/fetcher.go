package subscription

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchSubscription fetches a V2Ray subscription URL and returns parsed endpoints.
// It tries base64 decoding the body first, then falls back to plain text.
func FetchSubscription(urlStr string, timeout time.Duration) ([]Endpoint, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("fetcher: GET %s: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetcher: HTTP %d from %s", resp.StatusCode, urlStr)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetcher: read body: %w", err)
	}

	return ParseSubscription(strings.TrimSpace(string(body)))
}
