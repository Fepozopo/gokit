package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
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

	hasAnyAsset bool
}

// Sentinel errors for programmatic handling of update-check results.
var (
	// ErrNoReleases indicates the repository has no releases usable for update.
	ErrNoReleases = errors.New("no releases found")
	// ErrNoAsset indicates a release was found but no downloadable asset is present.
	ErrNoAsset = errors.New("no downloadable asset")
	// ErrNoPlatformAsset indicates the release has assets, but none match the current executable/runtime.
	ErrNoPlatformAsset = errors.New("no downloadable asset for current executable/platform")
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
// tag names and returns the highest semver it can find. Asset selection is
// matched against the current executable name plus the current GOOS/GOARCH.
func detectLatestRelease(repo string) (*Release, bool, error) {
	if repo == "" {
		return nil, false, fmt.Errorf("empty repo")
	}

	execBases, err := currentExecutableBaseCandidates(currentGOOS, currentGOARCH)
	if err != nil {
		return nil, false, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	body, err := getWithHeaders(defaultHTTPClient, apiURL, map[string]string{
		"Accept": "application/vnd.github.v3+json",
	})
	if err != nil {
		return nil, false, fmt.Errorf("github API request failed: %w", err)
	}

	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, false, fmt.Errorf("failed to decode github releases: %w", err)
	}

	candidates := make([]releaseCandidate, 0, len(releases))
	for _, r := range releases {
		candidate, ok := releaseCandidateFromGitHubRelease(r, execBases, currentGOOS, currentGOARCH)
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return nil, false, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].release.Version.GT(candidates[j].release.Version)
	})

	best := candidates[0].release
	return &best, true, nil
}

func parseReleaseVersion(tagName, releaseName string) (semver.Version, bool) {
	match := semverTagRegexp.FindString(tagName)
	if match == "" {
		match = semverTagRegexp.FindString(releaseName)
		if match == "" {
			return semver.Version{}, false
		}
	}
	v, err := semver.Parse(match)
	if err != nil {
		return semver.Version{}, false
	}
	return v, true
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

	currentSemVer, parseErr := semver.Parse(currentVersion)
	if parseErr != nil {
		slog.Warn("could not parse current version; treating as update available", "version", currentVersion, "error", parseErr)
		return UpdateCheckResult{
			Available: true,
			Latest:    latest,
			Err:       ErrCurrentVersionInvalid,
		}, nil
	}

	if !latest.Version.GT(currentSemVer) {
		return UpdateCheckResult{
			Available: false,
			Latest:    latest,
			Err:       nil,
		}, nil
	}

	if latest.AssetURL == "" {
		errState := ErrNoAsset
		if latest.hasAnyAsset {
			errState = ErrNoPlatformAsset
		}
		return UpdateCheckResult{
			Available: true,
			Latest:    latest,
			Err:       errState,
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
// trusted public key hex strings. GITHUB_TOKEN (if set) is used for authenticated
// requests. On Windows, replacing the running executable remains best-effort.
func Update(repo string, latest *Release, verify bool, trustedPubKeysHex []string) error {
	_ = repo
	if latest == nil {
		return fmt.Errorf("no release information provided")
	}
	if latest.AssetURL == "" {
		if latest.hasAnyAsset {
			return ErrNoPlatformAsset
		}
		return ErrNoAsset
	}
	if verify && len(trustedPubKeysHex) == 0 {
		return fmt.Errorf("verification requested but no trusted public keys were provided")
	}

	expected, err := expectedChecksumForRelease(latest, verify, trustedPubKeysHex)
	if err != nil {
		return err
	}

	exe, err := executablePath()
	if err != nil {
		return fmt.Errorf("could not locate executable: %w", err)
	}

	if err := installReleaseAsset(exe, latest, verify, expected); err != nil {
		return err
	}
	return restartUpdatedExecutable(exe)
}

func expectedChecksumForRelease(latest *Release, verify bool, trustedPubKeysHex []string) (string, error) {
	if !verify {
		return "", nil
	}
	if latest.ChecksumsURL == "" || latest.ChecksumsSigURL == "" {
		return "", fmt.Errorf("missing checksums or signature URL for release %s", latest.Version)
	}

	ckBody, err := getWithHeaders(defaultHTTPClient, latest.ChecksumsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed downloading checksums: %w", err)
	}
	sigBody, err := getWithHeaders(defaultHTTPClient, latest.ChecksumsSigURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed downloading checksums signature: %w", err)
	}
	if err := verifyChecksumsSignature(ckBody, string(sigBody), trustedPubKeysHex); err != nil {
		return "", fmt.Errorf("checksums signature verification failed: %w", err)
	}

	checks := parseChecksums(ckBody)
	expected, ok := checks[latest.AssetName]
	if (!ok || expected == "") && latest.AssetURL != "" {
		if fallback, fallbackOK := checks[path.Base(latest.AssetURL)]; fallbackOK {
			expected = fallback
			ok = true
		}
	}
	if !ok || expected == "" {
		return "", fmt.Errorf("no checksum entry found for asset %q in checksums.txt", latest.AssetName)
	}
	return expected, nil
}

func installReleaseAsset(exe string, latest *Release, verify bool, expected string) error {
	if err := downloadAndReplace(latest.AssetURL, exe, verify, expected); err != nil {
		if currentGOOS == "windows" {
			return fmt.Errorf("update install failed on Windows; replacing a running executable is best-effort and may require exiting before retrying: %w", err)
		}
		return fmt.Errorf("update failed: %w", err)
	}
	return nil
}

func restartUpdatedExecutable(exe string) error {
	if currentGOOS == "windows" {
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("updated executable but could not relaunch on Windows automatically; restart manually: %w", err)
		}
		return nil
	}

	argv := append([]string{exe}, os.Args[1:]...)
	if err := syscall.Exec(exe, argv, os.Environ()); err != nil {
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if startErr := cmd.Start(); startErr != nil {
			return fmt.Errorf("updated to new version but failed to restart automatically: execErr=%v startErr=%v", err, startErr)
		}
		return nil
	}
	return nil
}
