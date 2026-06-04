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

// doGet issues a GET request with the package's standard non-auth headers.
func doGet(client *http.Client, url string, extraHeaders map[string]string) (*http.Response, error) {
	return doGetWithAuth(client, url, extraHeaders, false)
}

// doGetWithGitHubToken issues a GET request and applies GITHUB_TOKEN auth when present.
func doGetWithGitHubToken(client *http.Client, url string, extraHeaders map[string]string) (*http.Response, error) {
	return doGetWithAuth(client, url, extraHeaders, true)
}

func doGetWithAuth(client *http.Client, url string, extraHeaders map[string]string, includeGitHubToken bool) (*http.Response, error) {
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
	applyBaseHeaders(req, extraHeaders)
	if includeGitHubToken {
		applyGitHubTokenAuth(req)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

// getWithHeaders performs a GET request and returns the full response body on success.
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

// getWithGitHubToken performs a GET request with GITHUB_TOKEN auth and returns the full response body on success.
func getWithGitHubToken(client *http.Client, url string, extraHeaders map[string]string) ([]byte, error) {
	resp, err := doGetWithGitHubToken(client, url, extraHeaders)
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

// applyBaseHeaders applies the package's standard non-auth headers.
func applyBaseHeaders(req *http.Request, extraHeaders map[string]string) {
	req.Header.Set("User-Agent", "gokit-update-checker")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
}

// applyGitHubTokenAuth applies GitHub token auth from the environment when present.
func applyGitHubTokenAuth(req *http.Request) {
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		// Reuse the same token-based auth for API and asset requests so callers can
		// benefit from higher GitHub rate limits and access private release assets.
		// The environment token intentionally wins over any Authorization header that
		// may have been supplied in extraHeaders.
		req.Header.Set("Authorization", "token "+token)
	}
}

// closeResponseBody closes an HTTP response body and logs any close failure.
func closeResponseBody(resp *http.Response, url string) {
	if resp == nil || resp.Body == nil {
		return
	}
	if cerr := resp.Body.Close(); cerr != nil {
		slog.Warn("response body close failed", "url", url, "err", cerr)
	}
}
