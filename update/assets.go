package update

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Name       string               `json:"name"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseCandidate struct {
	release Release
}

type assetMatch struct {
	score       int
	baseMatched bool
	generic     bool
	conflict    bool
}

var (
	currentGOOS     = runtime.GOOS
	currentGOARCH   = runtime.GOARCH
	executablePath  = os.Executable
	semverTagRegexp = regexp.MustCompile(`v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?`)
)

func releaseCandidateFromGitHubRelease(r githubRelease, execBases []string, goos, goarch string) (releaseCandidate, bool) {
	if r.Draft || r.Prerelease {
		return releaseCandidate{}, false
	}

	v, ok := parseReleaseVersion(r.TagName, r.Name)
	if !ok {
		return releaseCandidate{}, false
	}

	selectedAsset, hasAnyAsset := selectReleaseAsset(r.Assets, execBases, goos, goarch)
	checksumsURL, checksumsSigURL := checksumAssetURLs(r.Assets)

	return releaseCandidate{
		release: Release{
			Version:         v,
			AssetURL:        selectedAsset.BrowserDownloadURL,
			AssetName:       selectedAsset.Name,
			ChecksumsURL:    checksumsURL,
			ChecksumsSigURL: checksumsSigURL,
			hasAnyAsset:     hasAnyAsset,
		},
	}, true
}

func currentExecutableBaseCandidates(goos, goarch string) ([]string, error) {
	exe, err := executablePath()
	if err != nil {
		return nil, fmt.Errorf("could not locate executable: %w", err)
	}

	base := normalizeExecutableBase(filepath.Base(exe))
	if base == "" {
		return nil, fmt.Errorf("could not determine executable base name from %q", exe)
	}

	candidates := []string{base}
	if stripped := stripPlatformSuffix(base, goos, goarch); stripped != "" && stripped != base {
		candidates = append(candidates, stripped)
	}
	return uniqueStrings(candidates), nil
}

func normalizeExecutableBase(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if strings.HasSuffix(strings.ToLower(name), ".exe") {
		name = name[:len(name)-4]
	}
	return strings.ToLower(name)
}

func stripPlatformSuffix(base, goos, goarch string) string {
	for _, osAlias := range osAliases(goos) {
		for _, archAlias := range archAliases(goarch) {
			for _, sep := range []string{"-", "_", "."} {
				suffix := sep + osAlias + sep + archAlias
				if strings.HasSuffix(base, suffix) {
					return strings.TrimSuffix(base, suffix)
				}
			}
		}
	}
	return base
}

func checksumAssetURLs(assets []githubReleaseAsset) (checksumsURL, checksumsSigURL string) {
	for _, a := range assets {
		nameLower := strings.ToLower(a.Name)
		switch {
		case nameLower == "checksums.txt":
			checksumsURL = a.BrowserDownloadURL
		case isChecksumSignatureAsset(nameLower):
			checksumsSigURL = a.BrowserDownloadURL
		}
	}
	return checksumsURL, checksumsSigURL
}

func selectReleaseAsset(assets []githubReleaseAsset, execBases []string, goos, goarch string) (githubReleaseAsset, bool) {
	candidates := filterInstallableAssets(assets)
	if len(candidates) == 0 {
		return githubReleaseAsset{}, false
	}

	if exact, ok := selectExactAsset(candidates, execBases, goos, goarch); ok {
		return exact, true
	}

	considered := candidates
	if hasAnyBaseMatch(candidates, execBases) {
		considered = filterByBaseMatch(candidates, execBases)
	}

	if len(considered) == 1 {
		match := scoreAssetMatch(considered[0].Name, execBases, goos, goarch)
		if match.conflict {
			return githubReleaseAsset{}, true
		}
		if match.score > 0 || match.generic || match.baseMatched {
			return considered[0], true
		}
		return githubReleaseAsset{}, true
	}

	bestScore := -1
	bestIndex := -1
	ambiguous := false
	for i, asset := range considered {
		match := scoreAssetMatch(asset.Name, execBases, goos, goarch)
		if match.conflict || match.score <= 0 {
			continue
		}
		if match.score > bestScore {
			bestScore = match.score
			bestIndex = i
			ambiguous = false
			continue
		}
		if match.score == bestScore {
			ambiguous = true
		}
	}
	if bestIndex >= 0 && !ambiguous {
		return considered[bestIndex], true
	}
	return githubReleaseAsset{}, true
}

func filterInstallableAssets(assets []githubReleaseAsset) []githubReleaseAsset {
	out := make([]githubReleaseAsset, 0, len(assets))
	for _, a := range assets {
		nameLower := strings.ToLower(a.Name)
		if a.BrowserDownloadURL == "" || isMetadataAsset(nameLower) || isArchiveAsset(nameLower) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func selectExactAsset(assets []githubReleaseAsset, execBases []string, goos, goarch string) (githubReleaseAsset, bool) {
	expected := expectedExactAssetNames(execBases, goos, goarch)
	for _, asset := range assets {
		if _, ok := expected[strings.ToLower(asset.Name)]; ok {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}

func expectedExactAssetNames(execBases []string, goos, goarch string) map[string]struct{} {
	out := make(map[string]struct{})
	osVariants := osAliases(goos)
	archVariants := archAliases(goarch)
	separators := []string{"-", "_"}
	for _, base := range execBases {
		for _, osAlias := range osVariants {
			for _, archAlias := range archVariants {
				for _, sep := range separators {
					name := base + sep + osAlias + sep + archAlias
					out[name] = struct{}{}
					if goos == "windows" {
						out[name+".exe"] = struct{}{}
					}
				}
			}
		}
	}
	return out
}

func scoreAssetMatch(name string, execBases []string, goos, goarch string) assetMatch {
	baseMatched := assetMatchesAnyBase(name, execBases)
	osMatched, osMentioned := dimensionMatch(name, osAliases(goos), allOSAliases())
	archMatched, archMentioned := dimensionMatch(name, archAliases(goarch), allArchAliases())
	if (osMentioned && !osMatched) || (archMentioned && !archMatched) {
		return assetMatch{conflict: true}
	}

	score := 0
	if baseMatched {
		score += 4
	}
	if osMatched {
		score += 2
	}
	if archMatched {
		score++
	}
	return assetMatch{
		score:       score,
		baseMatched: baseMatched,
		generic:     !osMentioned && !archMentioned,
	}
}

func hasAnyBaseMatch(assets []githubReleaseAsset, execBases []string) bool {
	for _, asset := range assets {
		if assetMatchesAnyBase(asset.Name, execBases) {
			return true
		}
	}
	return false
}

func filterByBaseMatch(assets []githubReleaseAsset, execBases []string) []githubReleaseAsset {
	out := make([]githubReleaseAsset, 0, len(assets))
	for _, asset := range assets {
		if assetMatchesAnyBase(asset.Name, execBases) {
			out = append(out, asset)
		}
	}
	return out
}

func assetMatchesAnyBase(name string, execBases []string) bool {
	assetBase := normalizeExecutableBase(name)
	for _, base := range execBases {
		if assetBase == base || strings.HasPrefix(assetBase, base+"-") || strings.HasPrefix(assetBase, base+"_") || strings.HasPrefix(assetBase, base+".") {
			return true
		}
	}
	return false
}

func isMetadataAsset(nameLower string) bool {
	return nameLower == "checksums.txt" || isChecksumSignatureAsset(nameLower)
}

func isChecksumSignatureAsset(nameLower string) bool {
	switch nameLower {
	case "checksums.txt.sig", "checksums.sig", "checksums.txt.asc", "checksums.asc":
		return true
	default:
		return false
	}
}

func isArchiveAsset(nameLower string) bool {
	archiveSuffixes := []string{".zip", ".tar", ".tar.gz", ".tgz", ".tar.xz", ".txz", ".tar.bz2", ".tbz2", ".gz", ".xz", ".bz2"}
	for _, suffix := range archiveSuffixes {
		if strings.HasSuffix(nameLower, suffix) {
			return true
		}
	}
	return false
}

func dimensionMatch(name string, currentAliases []string, allAliases map[string][]string) (matched, mentioned bool) {
	for _, alias := range currentAliases {
		if containsPlatformToken(name, alias) {
			return true, true
		}
	}
	for _, aliases := range allAliases {
		for _, alias := range aliases {
			if containsPlatformToken(name, alias) {
				return false, true
			}
		}
	}
	return false, false
}

func containsPlatformToken(name, alias string) bool {
	pattern := fmt.Sprintf(`(^|[^a-z0-9])%s([^a-z0-9]|$)`, regexp.QuoteMeta(strings.ToLower(alias)))
	return regexp.MustCompile(pattern).MatchString(strings.ToLower(name))
}

func osAliases(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"darwin", "macos", "osx", "mac"}
	case "windows":
		return []string{"windows", "win32", "win64"}
	case "linux":
		return []string{"linux"}
	default:
		return []string{strings.ToLower(goos)}
	}
}

func archAliases(goarch string) []string {
	switch goarch {
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "386":
		return []string{"386", "x86", "i386", "i686"}
	case "arm":
		return []string{"arm", "armv6", "armv7"}
	default:
		return []string{strings.ToLower(goarch)}
	}
}

func allOSAliases() map[string][]string {
	return map[string][]string{
		"darwin":  osAliases("darwin"),
		"windows": osAliases("windows"),
		"linux":   osAliases("linux"),
	}
}

func allArchAliases() map[string][]string {
	return map[string][]string{
		"amd64": archAliases("amd64"),
		"arm64": archAliases("arm64"),
		"386":   archAliases("386"),
		"arm":   archAliases("arm"),
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
