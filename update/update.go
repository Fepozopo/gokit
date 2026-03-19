package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/Fepozopo/gokit/semver"
)

// Release is a minimal release descriptor used by detectLatestRelease.
type Release struct {
	Version   semver.Version
	AssetURL  string
	AssetName string
	// ChecksumsURL points to the checksums.txt asset for the release (containing
	// sha256 hashes for release assets). ChecksumsSigURL is the detached
	// ed25519 signature (hex) for the checksums file.
	ChecksumsURL    string
	ChecksumsSigURL string
}

// getWithHeaders performs a GET request using standard headers and optional extra headers.
// It sets User-Agent, includes Authorization from GITHUB_TOKEN if present, merges extraHeaders,
// and enforces response status checking and body reading.
func getWithHeaders(client *http.Client, url string, extraHeaders map[string]string) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("empty url")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating request: %w", err)
	}
	req.Header.Set("User-Agent", "gokit-update-checker")
	// merge extra headers (e.g. Accept)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("response body close failed", "url", url, "err", cerr)
		}
	}()
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

// Sentinel errors for programmatic handling of update-check results.
var (
	// ErrNoReleases indicates the repository has no releases usable for update.
	ErrNoReleases = errors.New("no releases found")
	// ErrNoAsset indicates a release was found but no downloadable asset is present.
	ErrNoAsset = errors.New("no downloadable asset")
	// ErrMissingChecksums indicates checksums or signature are missing for the release.
	ErrMissingChecksums = errors.New("missing checksums or signature")
	// ErrCurrentVersionInvalid indicates the current version string could not be parsed.
	ErrCurrentVersionInvalid = errors.New("could not parse current version")
)

// UpdateCheckResult represents the outcome of checking for updates.
// Use the Err field for programmatic inspection of special conditions
// (e.g. missing assets or an unparsable current version).
type UpdateCheckResult struct {
	Available bool
	Latest    *Release
	// Err is non-nil when the check resolved to a special state that callers
	// may want to inspect programmatically (e.g. ErrNoAsset). Note: network/API
	// errors are still returned via the function error return value.
	Err error
}

// detectLatestRelease queries the GitHub Releases API and returns the best-match
// release. It prefers published, non-prerelease releases with semver-compliant
// tag names and returns the highest semver it can find. If no suitable release
// is found it returns (nil, false, nil).
//
// This function sets recommended GitHub API headers and will honor an optional
// GITHUB_TOKEN environment variable for authenticated requests to increase rate
// limits.
func detectLatestRelease(repo string) (*Release, bool, error) {
	if repo == "" {
		return nil, false, fmt.Errorf("empty repo")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	client := defaultHTTPClient

	// Use shared helper to perform GET with standard headers and allow the GitHub Accept header.
	body, err := getWithHeaders(client, apiURL, map[string]string{
		"Accept": "application/vnd.github.v3+json",
	})
	if err != nil {
		return nil, false, fmt.Errorf("github API request failed: %w", err)
	}

	// Minimal struct to parse releases JSON
	var releases []struct {
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, false, fmt.Errorf("failed to decode github releases: %w", err)
	}

	type candidate struct {
		ver          semver.Version
		tag          string
		assetURL     string
		name         string
		checksumsURL string
		checksumsSig string
	}

	var candidates []candidate

	// regex to find semver substring like v1.2.3 or 1.2.3 inside tag name
	semverRe := regexp.MustCompile(`v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?`)

	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		tag := r.TagName
		match := semverRe.FindString(tag)
		if match == "" {
			// try the release name as a fallback
			match = semverRe.FindString(r.Name)
			if match == "" {
				continue
			}
		}
		v, perr := semver.Parse(match)
		if perr != nil {
			continue
		}
		assetURL := ""
		assetName := ""
		checksumsURL := ""
		checksumsSig := ""
		// find assets: prefer binary-like asset for download, and capture
		// checksums and signature assets if present.
		for _, a := range r.Assets {
			nameLower := strings.ToLower(a.Name)
			if nameLower == "checksums.txt" {
				checksumsURL = a.BrowserDownloadURL
				continue
			}
			if nameLower == "checksums.txt.sig" || nameLower == "checksums.sig" || nameLower == "checksums.txt.asc" || nameLower == "checksums.asc" {
				checksumsSig = a.BrowserDownloadURL
				continue
			}
			if strings.Contains(nameLower, "darwin") || strings.Contains(nameLower, "linux") || strings.Contains(nameLower, "windows") || strings.Contains(nameLower, "amd64") || strings.Contains(nameLower, "arm64") {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
				break
			}
			// fallback to first asset if nothing matches
			if assetURL == "" {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
			}
		}
		candidates = append(candidates, candidate{ver: v, tag: tag, assetURL: assetURL, name: assetName, checksumsURL: checksumsURL, checksumsSig: checksumsSig})
	}

	if len(candidates) == 0 {
		return nil, false, nil
	}

	// pick the highest semver
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ver.GT(candidates[j].ver)
	})
	best := candidates[0]

	// Build a Release struct
	r := &Release{
		Version:         best.ver,
		AssetURL:        best.assetURL,
		AssetName:       best.name,
		ChecksumsURL:    best.checksumsURL,
		ChecksumsSigURL: best.checksumsSig,
	}
	return r, true, nil
}

// CheckForUpdates checks for updates and returns a structured UpdateCheckResult.
//
// It does not return an error for normal states (such as "new release exists but missing asset"),
// those states are represented in UpdateCheckResult.Err. Errors are reserved for
// actual failures contacting the API or other unexpected failures.
func CheckForUpdates(currentVersion, repo string) (UpdateCheckResult, error) {
	latest, found, err := detectLatestRelease(repo)
	if err != nil {
		return UpdateCheckResult{}, fmt.Errorf("update check failed: %w", err)
	}
	if !found || latest == nil {
		return UpdateCheckResult{
			Available: false,
			Latest:    nil,
			Err:       ErrNoReleases,
		}, nil
	}

	// Try parsing current version; if parse fails we treat as unknown and indicate update available
	currentSemVer, parseErr := semver.Parse(currentVersion)
	if parseErr != nil {
		slog.Warn("could not parse current version; treating as update available", "version", currentVersion, "error", parseErr)
		return UpdateCheckResult{
			Available: true,
			Latest:    latest,
			Err:       ErrCurrentVersionInvalid,
		}, nil
	}

	// If latest is not strictly greater than current -> no update.
	if !latest.Version.GT(currentSemVer) {
		return UpdateCheckResult{
			Available: false,
			Latest:    latest,
			Err:       nil,
		}, nil
	}

	// Newer release exists. Return Err field when automatic update cannot be applied.
	if latest.AssetURL == "" {
		return UpdateCheckResult{
			Available: true,
			Latest:    latest,
			Err:       ErrNoAsset,
		}, nil
	}
	if latest.ChecksumsURL == "" || latest.ChecksumsSigURL == "" {
		return UpdateCheckResult{
			Available: true,
			Latest:    latest,
			Err:       ErrMissingChecksums,
		}, nil
	}

	return UpdateCheckResult{
		Available: true,
		Latest:    latest,
		Err:       nil,
	}, nil
}

// Update downloads and installs the given latest release. When verify is true,
// it will download checksums and signature and verify them using the provided
// trusted public key hex strings. HTTP handling uses timeouts and proper
// response status checks. GITHUB_TOKEN (if set) is used for authenticated
// requests for the checksums/signature downloads.
func Update(repo string, latest *Release, verify bool, trustedPubKeysHex []string) error {
	if latest == nil {
		return fmt.Errorf("no release information provided")
	}

	var expected string

	client := defaultHTTPClient

	// Use shared doGetWithHeaders helper for HTTP GETs.

	if verify {
		slog.Info("verifying release checksums signature")

		// Ensure checksums URLs are present
		if latest.ChecksumsURL == "" || latest.ChecksumsSigURL == "" {
			return fmt.Errorf("missing checksums or signature URL for release %s", latest.Version)
		}

		// Download checksums and signature using shared helper to ensure proper headers/timeouts.
		ckBody, err := getWithHeaders(client, latest.ChecksumsURL, nil)
		if err != nil {
			return fmt.Errorf("failed downloading checksums: %w", err)
		}
		sigBody, err := getWithHeaders(client, latest.ChecksumsSigURL, nil)
		if err != nil {
			return fmt.Errorf("failed downloading checksums signature: %w", err)
		}

		// Verify signature using embedded trusted public key(s).
		if err := verifyChecksumsSignature(ckBody, string(sigBody), trustedPubKeysHex); err != nil {
			return fmt.Errorf("checksums signature verification failed: %w", err)
		}

		// Parse checksums and find expected hash for the chosen asset.
		checks := parseChecksums(ckBody)
		var ok bool
		expected, ok = checks[latest.AssetName]
		if !ok || expected == "" {
			// Try fallback: use basename of asset URL
			if latest.AssetURL != "" {
				base := filepath.Base(latest.AssetURL)
				if v, ok2 := checks[base]; ok2 {
					expected = v
					ok = true
				}
			}
		}
		if !ok || expected == "" {
			return fmt.Errorf("no checksum entry found for asset %q in checksums.txt", latest.AssetName)
		}

		slog.Info("checksums signature valid; downloading and verifying artifact")
	} else {
		slog.Info("skipping checksum verification as requested")
		// expected remains empty when verification is disabled
		expected = ""
	}

	// Find executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not locate executable: %w", err)
	}

	// Use downloadAndReplace to fetch the release asset and replace the running executable.
	// downloadAndReplace is expected to handle large downloads and atomic replacement.
	if verify {
		if err := downloadAndReplace(latest.AssetURL, exe, true, expected); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
	} else {
		if err := downloadAndReplace(latest.AssetURL, exe, false, ""); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
	}

	// Attempt to restart the process by replacing the current process image on supported OSes.
	argv := append([]string{exe}, os.Args[1:]...)
	if runtimeGOOS() != "windows" {
		if err := syscall.Exec(exe, argv, os.Environ()); err != nil {
			// Exec only returns on error. Try a fallback of starting the new binary as a child process.
			cmd := exec.Command(exe, os.Args[1:]...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if startErr := cmd.Start(); startErr != nil {
				// If fallback also fails, return an error so the caller can decide how to handle restart.
				return fmt.Errorf("updated to new version but failed to restart automatically: execErr=%v startErr=%v", err, startErr)
			}
			// Successfully started the new process; return to caller.
			slog.Info("updated to version (started child process)", "version", latest.Version)
			return nil
		}
	} else {
		// On Windows, syscall.Exec is not applicable; attempt to start the new process.
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if startErr := cmd.Start(); startErr != nil {
			return fmt.Errorf("updated to new version but failed to restart automatically: startErr=%v", startErr)
		}
		slog.Info("updated to version (started child process on windows)", "version", latest.Version)
		return nil
	}

	// If Exec succeeds, this process is replaced and the following lines won't run.
	return nil
}
