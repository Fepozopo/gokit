package update

import (
	"encoding/json"
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
	"time"

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

// detectLatestRelease queries the GitHub Releases API and returns the best-match
// release. It prefers published, non-prerelease releases with semver-compliant
// tag names and returns the highest semver it can find. If no suitable release
// is found it returns (nil, false, nil).
func detectLatestRelease(repo string) (*Release, bool, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, false, fmt.Errorf("github API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("github API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed reading github response: %w", err)
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
		// normalize to start with v if missing (semver.Parse accepts both but keep consistent)
		verStr := match
		// semver.Parse expects no leading 'v' for github.com/blang/semver, but it supports v-prefixed too.
		v, perr := semver.Parse(verStr)
		if perr != nil {
			// try stripping leading 'v'
			verStr = strings.TrimPrefix(match, "v")
			v, perr = semver.Parse(verStr)
			if perr != nil {
				continue
			}
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

	// Build a selfupdate.Release-like struct (only include fields present in the actual type)
	r := &Release{
		Version:         best.ver,
		AssetURL:        best.assetURL,
		AssetName:       best.name,
		ChecksumsURL:    best.checksumsURL,
		ChecksumsSigURL: best.checksumsSig,
	}
	return r, true, nil
}

func CheckForUpdates(version, repo string) (bool, *Release, error) {
	// Use the GitHub API detector which is tolerant of tag naming.
	latest, found, err := detectLatestRelease(repo)
	slog.Info("current version", "version", version)
	if err != nil {
		return false, nil, fmt.Errorf("update check failed: %w", err)
	}
	if latest == nil {
		slog.Info("no release information available from GitHub")
	}

	if latest != nil {
		slog.Info("latest version", "version", latest.Version)
	}

	currentVer, parseErr := semver.Parse(version)
	if parseErr != nil {
		// If the built Version isn't valid semver, continue but warn.
		slog.Warn("could not parse current version", "version", version, "error", parseErr)
	}

	// No release found or nil result -> nothing to do.
	if !found || latest == nil {
		return false, nil, fmt.Errorf("no releases found for repo %q", repo)
	}

	// If same version -> up-to-date.
	if latest.Version.Equals(currentVer) {
		slog.Info("already running latest version", "version", currentVer)
		return false, latest, nil
	}

	// If we don't have an asset URL, cannot update automatically.
	if latest.AssetURL == "" {
		return true, latest, fmt.Errorf("new version %s available but no downloadable asset", latest.Version)
	}

	// Signed checksums are not present for the release.
	if latest.ChecksumsURL == "" || latest.ChecksumsSigURL == "" {
		slog.Warn("new version available but missing checksums or signature", "version", latest.Version)
		return true, latest, fmt.Errorf("new version available but missing checksums or signature")
	}

	// Prompt the user to confirm updating.
	slog.Info("A new version (%s) is available.", latest.Version)
	return true, latest, nil
}

func Update(repo string, latest *Release, verify bool, trustedPubKeysHex []string) error {
	slog.Info("verifying release checksums signature")
	// Download checksums and signature
	ckResp, err := http.Get(latest.ChecksumsURL)
	if err != nil {
		return fmt.Errorf("failed downloading checksums: %w", err)
	}
	ckBody, err := io.ReadAll(ckResp.Body)
	_ = ckResp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed reading checksums: %w", err)
	}
	sigResp, err := http.Get(latest.ChecksumsSigURL)
	if err != nil {
		return fmt.Errorf("failed downloading checksums signature: %w", err)
	}
	sigBody, err := io.ReadAll(sigResp.Body)
	_ = sigResp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed reading checksums signature: %w", err)
	}

	// Verify signature using embedded trusted public key(s).
	if err := verifyChecksumsSignature(ckBody, string(sigBody), trustedPubKeysHex); err != nil {
		return fmt.Errorf("checksums signature verification failed: %w", err)
	}

	// Parse checksums and find expected hash for the chosen asset.
	checks := parseChecksums(ckBody)
	expected, ok := checks[latest.AssetName]
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
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not locate executable: %w", err)
	}

	if verify {
		// Download, verify checksum, and atomically replace the executable.
		if err := downloadAndReplace(latest.AssetURL, exe, true, expected); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
	} else {
		// Just download and replace without verifying checksum (not recommended).
		// Pass an empty expected hash since verification is disabled.
		if err := downloadAndReplace(latest.AssetURL, exe, false, ""); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}
	}

	// Attempt to restart the process by replacing the current process image.
	argv := append([]string{exe}, os.Args[1:]...)
	if err := syscall.Exec(exe, argv, os.Environ()); err != nil {
		// Exec only returns on error. Try a fallback of starting the new binary as a child process.
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if startErr := cmd.Start(); startErr != nil {
			// If fallback also fails, report success but instruct user to restart manually.
			slog.Info("updated to new version but failed to restart automatically", "version", latest.Version, "execErr", err, "startErr", startErr)
			slog.Info("please restart the application manually")
			return nil
		}
		// Successfully started the new process; exit the current one.
		slog.Info("updated to version %s successfully", latest.Version)
		os.Exit(0)
	}

	// If Exec succeeds, this process is replaced and the following lines won't run.
	return nil
}
