package update

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
	"syscall"

	"github.com/Fepozopo/gokit/semver"
)

// ReleaseAssetStatus describes how asset selection resolved for a release.
type ReleaseAssetStatus string

const (
	// ReleaseAssetStatusUnknown indicates the release did not record an explicit asset-selection outcome.
	ReleaseAssetStatusUnknown ReleaseAssetStatus = ""
	// ReleaseAssetStatusSelected indicates a directly installable asset was selected for the current executable/platform.
	ReleaseAssetStatusSelected ReleaseAssetStatus = "selected"
	// ReleaseAssetStatusNoInstallableAsset indicates the release had no directly installable asset at all.
	ReleaseAssetStatusNoInstallableAsset ReleaseAssetStatus = "no_installable_asset"
	// ReleaseAssetStatusNoPlatformMatch indicates the release had installable assets, but none matched the current executable/platform.
	ReleaseAssetStatusNoPlatformMatch ReleaseAssetStatus = "no_platform_match"
)

// Release is a minimal release descriptor used by detectLatestRelease.
type Release struct {
	Version   semver.Version
	AssetURL  string
	AssetName string
	// AssetStatus reports how release asset selection resolved for the current
	// executable and platform.
	AssetStatus ReleaseAssetStatus
	// ChecksumsURL points to the checksums.txt asset for the release (containing
	// sha256 hashes for release assets). ChecksumsSigURL is the detached
	// ed25519 signature (hex) for the checksums file.
	ChecksumsURL    string
	ChecksumsSigURL string
}

// CheckStatus describes the outcome of an update check.
type CheckStatus string

const (
	// CheckStatusUpToDate indicates the current version is not older than the latest usable release.
	CheckStatusUpToDate CheckStatus = "up_to_date"
	// CheckStatusNoReleases indicates the repository has no usable releases.
	CheckStatusNoReleases CheckStatus = "no_releases"
	// CheckStatusCurrentVersionInvalid indicates the current version could not be parsed, so an update is treated as available.
	CheckStatusCurrentVersionInvalid CheckStatus = "current_version_invalid"
	// CheckStatusUpdateAvailable indicates a newer release is available and has the required asset and checksum metadata.
	CheckStatusUpdateAvailable CheckStatus = "update_available"
	// CheckStatusUpdateAvailableNoAsset indicates a newer release exists but has no directly installable asset.
	CheckStatusUpdateAvailableNoAsset CheckStatus = "update_available_no_asset"
	// CheckStatusUpdateAvailableNoPlatformAsset indicates a newer release exists but no asset matches the current executable/platform.
	CheckStatusUpdateAvailableNoPlatformAsset CheckStatus = "update_available_no_platform_asset"
	// CheckStatusUpdateAvailableMissingChecksums indicates a newer release exists but is missing checksums or signature metadata.
	CheckStatusUpdateAvailableMissingChecksums CheckStatus = "update_available_missing_checksums"
)

// UpdateCheckResult represents the outcome of checking for updates.
type UpdateCheckResult struct {
	// Status is the primary programmatic result of the update check.
	Status CheckStatus
	// Available reports whether a newer release exists, even if it is not currently installable.
	Available bool
	// Latest holds the newest usable release that was discovered, when one exists.
	Latest *Release
}

// detectLatestRelease queries the GitHub Releases API and returns the best-matching release.
func detectLatestRelease(repo string) (*Release, bool, error) {
	if repo == "" {
		return nil, false, fmt.Errorf("empty repo")
	}

	execBases, err := currentExecutableBaseCandidates(currentGOOS, currentGOARCH)
	if err != nil {
		return nil, false, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	body, err := getWithGitHubToken(defaultHTTPClient, apiURL, map[string]string{
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

// parseReleaseVersion extracts and parses a semantic version from a release tag or name.
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

// CheckForUpdates checks GitHub releases for a newer version of the current executable.
//
// Normal domain outcomes such as "already up to date", "no releases", or
// "new release exists but is missing an installable asset" are reported via the
// returned UpdateCheckResult.Status. The returned error is reserved for
// operational failures such as HTTP, decoding, or other unexpected problems that
// prevented the check from completing.
func CheckForUpdates(currentVersion, repo string) (UpdateCheckResult, error) {
	latest, found, err := detectLatestRelease(repo)
	if err != nil {
		return UpdateCheckResult{}, fmt.Errorf("update check failed: %w", err)
	}
	if !found || latest == nil {
		return newUpdateCheckResult(CheckStatusNoReleases, nil), nil
	}

	currentSemVer, parseErr := semver.Parse(currentVersion)
	if parseErr != nil {
		slog.Warn("could not parse current version; treating as update available", "version", currentVersion, "error", parseErr)
		return newUpdateCheckResult(CheckStatusCurrentVersionInvalid, latest), nil
	}

	if !latest.Version.GT(currentSemVer) {
		return newUpdateCheckResult(CheckStatusUpToDate, latest), nil
	}

	if latest.AssetURL == "" {
		switch latest.AssetStatus {
		case ReleaseAssetStatusNoPlatformMatch:
			return newUpdateCheckResult(CheckStatusUpdateAvailableNoPlatformAsset, latest), nil
		default:
			return newUpdateCheckResult(CheckStatusUpdateAvailableNoAsset, latest), nil
		}
	}
	if latest.ChecksumsURL == "" || latest.ChecksumsSigURL == "" {
		return newUpdateCheckResult(CheckStatusUpdateAvailableMissingChecksums, latest), nil
	}

	return newUpdateCheckResult(CheckStatusUpdateAvailable, latest), nil
}

// newUpdateCheckResult builds the canonical UpdateCheckResult for a completed
// check so all call sites derive the Available flag from Status consistently.
func newUpdateCheckResult(status CheckStatus, latest *Release) UpdateCheckResult {
	return UpdateCheckResult{
		Status:    status,
		Available: statusIndicatesAvailable(status),
		Latest:    latest,
	}
}

// statusIndicatesAvailable reports whether a CheckStatus means a newer release
// exists, even if that release cannot yet be installed automatically.
func statusIndicatesAvailable(status CheckStatus) bool {
	switch status {
	case CheckStatusCurrentVersionInvalid,
		CheckStatusUpdateAvailable,
		CheckStatusUpdateAvailableNoAsset,
		CheckStatusUpdateAvailableNoPlatformAsset,
		CheckStatusUpdateAvailableMissingChecksums:
		return true
	default:
		return false
	}
}

// Update downloads, verifies, installs, and restarts into the given latest release.
//
// The caller is expected to pass the Release returned by CheckForUpdates. When
// the release metadata does not describe an installable asset for the current
// executable or platform, Update returns a descriptive error instead of
// attempting any download or replacement work.
func Update(latest *Release, verify bool, trustedPubKeysHex []string) error {
	if latest == nil {
		return fmt.Errorf("no release information provided")
	}
	if latest.AssetURL == "" {
		switch latest.AssetStatus {
		case ReleaseAssetStatusNoPlatformMatch:
			return fmt.Errorf("latest release has no asset matching the current executable/platform")
		default:
			return fmt.Errorf("latest release has no directly installable asset")
		}
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

// expectedChecksumForRelease resolves the checksum that should match the selected release asset.
func expectedChecksumForRelease(latest *Release, verify bool, trustedPubKeysHex []string) (string, error) {
	if !verify {
		return "", nil
	}
	if latest.ChecksumsURL == "" || latest.ChecksumsSigURL == "" {
		return "", fmt.Errorf("missing checksums or signature URL for release %s", latest.Version)
	}

	ckBody, err := getWithGitHubToken(defaultHTTPClient, latest.ChecksumsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed downloading checksums: %w", err)
	}
	sigBody, err := getWithGitHubToken(defaultHTTPClient, latest.ChecksumsSigURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed downloading checksums signature: %w", err)
	}
	// Verify the detached signature over the raw checksums.txt payload before
	// trusting any hash extracted from that file.
	if err := verifyChecksumsSignature(ckBody, string(sigBody), trustedPubKeysHex); err != nil {
		return "", fmt.Errorf("checksums signature verification failed: %w", err)
	}

	checks := parseChecksums(ckBody)
	expected, ok := checks[latest.AssetName]
	if (!ok || expected == "") && latest.AssetURL != "" {
		// Some projects list the downloadable basename in checksums.txt even when
		// the selected asset name came from a different display field.
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

// installReleaseAsset downloads and installs the selected release asset over exe.
func installReleaseAsset(exe string, latest *Release, verify bool, expected string) error {
	if err := downloadAndReplace(latest.AssetURL, exe, verify, expected); err != nil {
		if currentGOOS == "windows" {
			// Replacing a running executable can fail on Windows depending on how the
			// file is locked, so installation remains a best-effort operation there.
			return fmt.Errorf("update install failed on Windows; replacing a running executable is best-effort and may require exiting before retrying: %w", err)
		}
		return fmt.Errorf("update failed: %w", err)
	}
	return nil
}

// restartUpdatedExecutable attempts to launch the updated executable after installation.
func restartUpdatedExecutable(exe string) error {
	if currentGOOS == "windows" {
		// Windows cannot replace the current process image with syscall.Exec, so the
		// best available option is to start a new process and let the caller exit.
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
