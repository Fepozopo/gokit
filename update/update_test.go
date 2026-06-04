package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fepozopo/gokit/semver"
)

// roundTripperFunc adapts a function to implement http.RoundTripper in tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip executes f for req.
func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// rewriteGitHubClient returns an HTTP client that rewrites GitHub URLs to the test server.
func rewriteGitHubClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			rewritten := *clone.URL
			rewritten.Scheme = baseURL.Scheme
			rewritten.Host = baseURL.Host
			clone.URL = &rewritten
			return server.Client().Transport.RoundTrip(clone)
		}),
	}
}

// setUpdateTestGlobals overrides package-level globals for tests and restores them on cleanup.
func setUpdateTestGlobals(t *testing.T, goos, goarch, exe string, client *http.Client) {
	t.Helper()

	oldGOOS := currentGOOS
	oldGOARCH := currentGOARCH
	oldExecutablePath := executablePath
	oldHTTPClient := defaultHTTPClient

	currentGOOS = goos
	currentGOARCH = goarch
	executablePath = func() (string, error) {
		return exe, nil
	}
	if client != nil {
		defaultHTTPClient = client
	}

	t.Cleanup(func() {
		currentGOOS = oldGOOS
		currentGOARCH = oldGOARCH
		executablePath = oldExecutablePath
		defaultHTTPClient = oldHTTPClient
	})
}

// TestSelectReleaseAssetPrefersExactExecutableAndPlatform verifies exact asset matches win over other candidates.
func TestSelectReleaseAssetPrefersExactExecutableAndPlatform(t *testing.T) {
	assets := []githubReleaseAsset{
		{Name: "worker-linux-amd64", BrowserDownloadURL: "https://example.invalid/worker-linux-amd64"},
		{Name: "myapp-darwin-arm64", BrowserDownloadURL: "https://example.invalid/myapp-darwin-arm64"},
		{Name: "myapp-linux-amd64", BrowserDownloadURL: "https://example.invalid/myapp-linux-amd64"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.invalid/checksums.txt"},
	}

	selection := selectReleaseAsset(assets, []string{"myapp"}, "linux", "amd64")
	if selection.status != ReleaseAssetStatusSelected {
		t.Fatalf("asset status = %q, want %q", selection.status, ReleaseAssetStatusSelected)
	}
	if selection.asset.Name != "myapp-linux-amd64" {
		t.Fatalf("selected asset = %q, want %q", selection.asset.Name, "myapp-linux-amd64")
	}
}

// TestSelectReleaseAssetDoesNotGuessDifferentExecutable verifies mismatched executable names are rejected.
func TestSelectReleaseAssetDoesNotGuessDifferentExecutable(t *testing.T) {
	assets := []githubReleaseAsset{
		{Name: "worker-linux-amd64", BrowserDownloadURL: "https://example.invalid/worker-linux-amd64"},
	}

	selection := selectReleaseAsset(assets, []string{"myapp"}, "linux", "amd64")
	if selection.status != ReleaseAssetStatusNoPlatformMatch {
		t.Fatalf("asset status = %q, want %q", selection.status, ReleaseAssetStatusNoPlatformMatch)
	}
	if selection.asset.Name != "" || selection.asset.BrowserDownloadURL != "" {
		t.Fatalf("expected no asset to be selected, got %#v", selection.asset)
	}
}

// TestDoGetPreservesCallerAuthorizationHeaderWithoutGitHubToken verifies generic requests keep caller auth when no env token is set.
func TestDoGetPreservesCallerAuthorizationHeaderWithoutGitHubToken(t *testing.T) {
	var authHeader string
	var userAgent string
	var customHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		userAgent = r.Header.Get("User-Agent")
		customHeader = r.Header.Get("X-Custom")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	t.Setenv("GITHUB_TOKEN", "")

	resp, err := doGet(server.Client(), server.URL, map[string]string{
		"Authorization": "Bearer caller-token",
		"X-Custom":      "present",
	})
	if err != nil {
		t.Fatalf("doGet returned error: %v", err)
	}
	defer closeResponseBody(resp, server.URL)

	if authHeader != "Bearer caller-token" {
		t.Fatalf("authorization header = %q, want %q", authHeader, "Bearer caller-token")
	}
	if userAgent != "gokit-update-checker" {
		t.Fatalf("user-agent = %q, want %q", userAgent, "gokit-update-checker")
	}
	if customHeader != "present" {
		t.Fatalf("x-custom header = %q, want %q", customHeader, "present")
	}
}

// TestDoGetWithGitHubTokenOverridesCallerAuthorizationHeader verifies env token auth wins for authenticated update requests.
func TestDoGetWithGitHubTokenOverridesCallerAuthorizationHeader(t *testing.T) {
	var authHeader string
	var customHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		customHeader = r.Header.Get("X-Custom")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	t.Setenv("GITHUB_TOKEN", "secret-token")

	resp, err := doGetWithGitHubToken(server.Client(), server.URL, map[string]string{
		"Authorization": "Bearer caller-token",
		"X-Custom":      "present",
	})
	if err != nil {
		t.Fatalf("doGetWithGitHubToken returned error: %v", err)
	}
	defer closeResponseBody(resp, server.URL)

	if authHeader != "token secret-token" {
		t.Fatalf("authorization header = %q, want %q", authHeader, "token secret-token")
	}
	if customHeader != "present" {
		t.Fatalf("x-custom header = %q, want %q", customHeader, "present")
	}
}

// TestDetectLatestReleaseUsesGithubTokenAndSelectsLatestStableMatchingAsset verifies auth and release selection behavior.
func TestDetectLatestReleaseUsesGithubTokenAndSelectsLatestStableMatchingAsset(t *testing.T) {
	var authHeader string

	releasesJSON := `[
				{
					"tag_name": "v2.0.0-beta.1",
					"prerelease": true,
					"assets": [
						{"name": "myapp-linux-amd64", "browser_download_url": "https://example.invalid/prerelease-bin"}
					]
				},
				{
					"tag_name": "v1.10.0",
					"assets": [
						{"name": "worker-linux-amd64", "browser_download_url": "https://example.invalid/worker-linux-amd64"},
						{"name": "myapp-linux-amd64", "browser_download_url": "https://example.invalid/myapp-linux-amd64"},
						{"name": "checksums.txt", "browser_download_url": "https://example.invalid/checksums.txt"},
						{"name": "checksums.txt.sig", "browser_download_url": "https://example.invalid/checksums.txt.sig"}
					]
				},
				{
					"tag_name": "v1.9.0",
					"assets": [
						{"name": "myapp-linux-amd64", "browser_download_url": "https://example.invalid/myapp-1.9.0-linux-amd64"}
					]
				}
			]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/repos/owner/repo/releases" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, releasesJSON)
	}))
	defer server.Close()

	t.Setenv("GITHUB_TOKEN", "secret-token")
	setUpdateTestGlobals(t, "linux", "amd64", "/tmp/myapp", rewriteGitHubClient(t, server))

	latest, found, err := detectLatestRelease("owner/repo")
	if err != nil {
		t.Fatalf("detectLatestRelease returned error: %v", err)
	}
	if !found {
		t.Fatal("expected a release to be found")
	}
	if latest == nil {
		t.Fatal("expected release details, got nil")
	}
	if authHeader != "token secret-token" {
		t.Fatalf("authorization header = %q, want %q", authHeader, "token secret-token")
	}
	if latest.Version.String() != "1.10.0" {
		t.Fatalf("latest version = %q, want %q", latest.Version.String(), "1.10.0")
	}
	if latest.AssetName != "myapp-linux-amd64" {
		t.Fatalf("asset name = %q, want %q", latest.AssetName, "myapp-linux-amd64")
	}
	if latest.AssetStatus != ReleaseAssetStatusSelected {
		t.Fatalf("asset status = %q, want %q", latest.AssetStatus, ReleaseAssetStatusSelected)
	}
}

// TestCheckForUpdatesReturnsNoPlatformAssetStatus verifies platform mismatches are surfaced explicitly.
func TestCheckForUpdatesReturnsNoPlatformAssetStatus(t *testing.T) {
	releasesJSON := `[
				{
					"tag_name": "v1.2.0",
					"assets": [
						{"name": "worker-linux-amd64", "browser_download_url": "https://example.invalid/worker-linux-amd64"},
						{"name": "checksums.txt", "browser_download_url": "https://example.invalid/checksums.txt"},
						{"name": "checksums.txt.sig", "browser_download_url": "https://example.invalid/checksums.txt.sig"}
					]
				}
			]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, releasesJSON)
	}))
	defer server.Close()

	setUpdateTestGlobals(t, "linux", "amd64", "/tmp/myapp", rewriteGitHubClient(t, server))

	res, err := CheckForUpdates("v1.0.0", "owner/repo")
	if err != nil {
		t.Fatalf("CheckForUpdates returned error: %v", err)
	}
	if !res.Available {
		t.Fatal("expected update to be available")
	}
	if res.Status != CheckStatusUpdateAvailableNoPlatformAsset {
		t.Fatalf("result status = %q, want %q", res.Status, CheckStatusUpdateAvailableNoPlatformAsset)
	}
}

// TestExpectedChecksumForReleaseVerifiesSignatureAndUsesAuth verifies signed checksum lookup uses authenticated requests.
func TestExpectedChecksumForReleaseVerifiesSignatureAndUsesAuth(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	assetBody := []byte("binary payload")
	sum := sha256.Sum256(assetBody)
	expectedHash := fmt.Sprintf("%x", sum[:])
	checksumsBody := []byte(fmt.Sprintf("%s  myapp-linux-amd64\n", expectedHash))
	sigHex := hex.EncodeToString(ed25519.Sign(priv, checksumsBody))

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("Authorization"); got != "token secret-token" {
			t.Fatalf("authorization header = %q, want %q", got, "token secret-token")
		}
		switch r.URL.Path {
		case "/checksums.txt":
			_, _ = w.Write(checksumsBody)
		case "/checksums.txt.sig":
			_, _ = io.WriteString(w, sigHex)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("GITHUB_TOKEN", "secret-token")
	setUpdateTestGlobals(t, "linux", "amd64", "/tmp/myapp", server.Client())

	latest := &Release{
		Version:         semver.Version{Major: 1, Minor: 2, Patch: 3},
		AssetName:       "myapp-linux-amd64",
		AssetURL:        server.URL + "/myapp-linux-amd64",
		ChecksumsURL:    server.URL + "/checksums.txt",
		ChecksumsSigURL: server.URL + "/checksums.txt.sig",
	}

	expected, err := expectedChecksumForRelease(latest, true, []string{hex.EncodeToString(pub)})
	if err != nil {
		t.Fatalf("expectedChecksumForRelease returned error: %v", err)
	}
	if expected != expectedHash {
		t.Fatalf("expected checksum = %q, want %q", expected, expectedHash)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
}

// TestExpectedChecksumForReleaseFailsInvalidSignature verifies invalid checksum signatures are rejected.
func TestExpectedChecksumForReleaseFailsInvalidSignature(t *testing.T) {
	_, goodPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	checksumsBody := []byte("1234  myapp-linux-amd64\n")
	badSigHex := hex.EncodeToString(ed25519.Sign(otherPriv, checksumsBody))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checksums.txt":
			_, _ = w.Write(checksumsBody)
		case "/checksums.txt.sig":
			_, _ = io.WriteString(w, badSigHex)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	setUpdateTestGlobals(t, "linux", "amd64", "/tmp/myapp", server.Client())

	latest := &Release{
		Version:         semver.Version{Major: 1, Minor: 2, Patch: 3},
		AssetName:       "myapp-linux-amd64",
		AssetURL:        server.URL + "/myapp-linux-amd64",
		ChecksumsURL:    server.URL + "/checksums.txt",
		ChecksumsSigURL: server.URL + "/checksums.txt.sig",
	}

	_, err = expectedChecksumForRelease(latest, true, []string{hex.EncodeToString(goodPriv.Public().(ed25519.PublicKey))})
	if err == nil {
		t.Fatal("expected signature verification to fail")
	}
	if !strings.Contains(err.Error(), "checksums signature verification failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDownloadAndReplaceUsesAuthForAssetDownloads verifies asset downloads inherit GitHub token auth.
func TestDownloadAndReplaceUsesAuthForAssetDownloads(t *testing.T) {
	assetBody := []byte("downloaded binary")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token secret-token" {
			t.Fatalf("authorization header = %q, want %q", got, "token secret-token")
		}
		if r.URL.Path != "/asset" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_, _ = w.Write(assetBody)
	}))
	defer server.Close()

	t.Setenv("GITHUB_TOKEN", "secret-token")
	setUpdateTestGlobals(t, currentGOOS, currentGOARCH, "/tmp/myapp", server.Client())

	dest := filepath.Join(t.TempDir(), "myapp")
	if err := downloadAndReplace(server.URL+"/asset", dest, false, ""); err != nil {
		t.Fatalf("downloadAndReplace returned error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read replaced file: %v", err)
	}
	if string(got) != string(assetBody) {
		t.Fatalf("replaced file content = %q, want %q", string(got), string(assetBody))
	}
}

// TestUpdateRejectsReleaseWithoutPlatformMatch verifies Update fails fast when the release metadata
// already indicates that no asset matches the current executable/platform.
func TestUpdateRejectsReleaseWithoutPlatformMatch(t *testing.T) {
	latest := &Release{
		Version:     semver.Version{Major: 1, Minor: 2, Patch: 3},
		AssetStatus: ReleaseAssetStatusNoPlatformMatch,
	}

	err := Update(latest, false, nil)
	if err == nil {
		t.Fatal("expected Update to reject a release without a matching asset")
	}
	if !strings.Contains(err.Error(), "no asset matching the current executable/platform") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDownloadAndReplaceRejectsChecksumMismatch verifies checksum validation fails before replacement.
func TestDownloadAndReplaceRejectsChecksumMismatch(t *testing.T) {
	assetBody := []byte("downloaded binary")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(assetBody)
	}))
	defer server.Close()

	setUpdateTestGlobals(t, currentGOOS, currentGOARCH, "/tmp/myapp", server.Client())

	dest := filepath.Join(t.TempDir(), "myapp")
	err := downloadAndReplace(server.URL, dest, true, "deadbeef")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
