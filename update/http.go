package update

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// defaultHTTPClient is used by helper download functions to ensure timeouts.
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

func doGet(client *http.Client, url string, extraHeaders map[string]string) (*http.Response, error) {
	if url == "" {
		return nil, fmt.Errorf("empty url")
	}
	if client == nil {
		client = defaultHTTPClient
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating request: %w", err)
	}
	applyCommonHeaders(req, extraHeaders)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

// getWithHeaders performs a GET request using standard headers and optional extra headers.
// It sets User-Agent, includes Authorization from GITHUB_TOKEN if present, merges extraHeaders,
// and enforces response status checking and body reading.
func getWithHeaders(client *http.Client, url string, extraHeaders map[string]string) ([]byte, error) {
	resp, err := doGet(client, url, extraHeaders)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp, url)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading response body: %w", err)
	}
	return body, nil
}

func applyCommonHeaders(req *http.Request, extraHeaders map[string]string) {
	req.Header.Set("User-Agent", "gokit-update-checker")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
}

func closeResponseBody(resp *http.Response, url string) {
	if resp == nil || resp.Body == nil {
		return
	}
	if cerr := resp.Body.Close(); cerr != nil {
		slog.Warn("response body close failed", "url", url, "err", cerr)
	}
}
