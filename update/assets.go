package update

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// githubRelease mirrors the subset of the GitHub Releases API response that this package uses.
type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Name       string               `json:"name"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

// githubReleaseAsset mirrors the subset of a GitHub release asset that this package uses.
type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// releaseCandidate wraps a parsed release that survived filtering and asset selection.
type releaseCandidate struct {
	release Release
}

// releaseAssetSelection records the selected asset, if any, along with the reason selection resolved the way it did.
type releaseAssetSelection struct {
	asset  githubReleaseAsset
	status ReleaseAssetStatus
}

// assetMatch describes how well an asset name matches the current executable and platform.
type assetMatch struct {
	score       int
	baseMatched bool
	generic     bool
	conflict    bool
}

var semverTagRegexp = regexp.MustCompile(`v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?`)

// releaseCandidateFromGitHubRelease converts a GitHub release into a candidate release.
func releaseCandidateFromGitHubRelease(r githubRelease, execBases []string, goos, goarch string) (releaseCandidate, bool) {
	if r.Draft || r.Prerelease {
		return releaseCandidate{}, false
	}

	v, ok := parseReleaseVersion(r.TagName, r.Name)
	if !ok {
		return releaseCandidate{}, false
	}

	selection := selectReleaseAsset(r.Assets, execBases, goos, goarch)
	checksumsURL, checksumsSigURL := checksumAssetURLs(r.Assets)

	return releaseCandidate{
		release: Release{
			Version:         v,
			AssetURL:        selection.asset.BrowserDownloadURL,
			AssetName:       selection.asset.Name,
			AssetStatus:     selection.status,
			ChecksumsURL:    checksumsURL,
			ChecksumsSigURL: checksumsSigURL,
		},
	}, true
}

// currentExecutableBaseCandidates derives the executable base names that should
// be matched against release assets for the supplied platform.
//
// The executablePath function is injected so callers can substitute a fake
// executable location during tests instead of relying on mutable package state.
func currentExecutableBaseCandidates(goos, goarch string, executablePath func() (string, error)) ([]string, error) {
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

// normalizeExecutableBase normalizes an executable or asset name for matching.
func normalizeExecutableBase(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if strings.HasSuffix(strings.ToLower(name), ".exe") {
		name = name[:len(name)-4]
	}
	return strings.ToLower(name)
}

// stripPlatformSuffix removes a trailing os-arch suffix from a normalized executable base.
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

// checksumAssetURLs returns the checksum and checksum-signature asset URLs for a release.
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

// selectReleaseAsset chooses the best installable asset for the current executable and platform.
func selectReleaseAsset(assets []githubReleaseAsset, execBases []string, goos, goarch string) releaseAssetSelection {
	candidates := filterInstallableAssets(assets)
	if len(candidates) == 0 {
		return releaseAssetSelection{status: ReleaseAssetStatusNoInstallableAsset}
	}

	// Prefer an exact "<binary>-<os>-<arch>" style name before applying heuristics.
	if exact, ok := selectExactAsset(candidates, execBases, goos, goarch); ok {
		return releaseAssetSelection{asset: exact, status: ReleaseAssetStatusSelected}
	}

	considered := filterByBaseMatch(candidates, execBases)
	if len(considered) == 0 {
		return releaseAssetSelection{status: ReleaseAssetStatusNoPlatformMatch}
	}

	if len(considered) == 1 {
		match := scoreAssetMatch(considered[0].Name, execBases, goos, goarch)
		if match.conflict {
			return releaseAssetSelection{status: ReleaseAssetStatusNoPlatformMatch}
		}
		if match.score > 0 || match.generic || match.baseMatched {
			return releaseAssetSelection{asset: considered[0], status: ReleaseAssetStatusSelected}
		}
		return releaseAssetSelection{status: ReleaseAssetStatusNoPlatformMatch}
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
	// If multiple assets tie for the best heuristic match, refuse to guess.
	if bestIndex >= 0 && !ambiguous {
		return releaseAssetSelection{asset: considered[bestIndex], status: ReleaseAssetStatusSelected}
	}
	return releaseAssetSelection{status: ReleaseAssetStatusNoPlatformMatch}
}

// filterInstallableAssets removes release assets that this updater cannot install directly.
func filterInstallableAssets(assets []githubReleaseAsset) []githubReleaseAsset {
	out := make([]githubReleaseAsset, 0, len(assets))
	for _, a := range assets {
		nameLower := strings.ToLower(a.Name)
		// This updater swaps in a single executable file. Metadata assets and archives
		// are intentionally ignored because this package does not unpack releases.
		if a.BrowserDownloadURL == "" || isMetadataAsset(nameLower) || isArchiveAsset(nameLower) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// selectExactAsset returns an asset whose name exactly matches one of the expected platform variants.
func selectExactAsset(assets []githubReleaseAsset, execBases []string, goos, goarch string) (githubReleaseAsset, bool) {
	expected := expectedExactAssetNames(execBases, goos, goarch)
	for _, asset := range assets {
		if _, ok := expected[strings.ToLower(asset.Name)]; ok {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}

// expectedExactAssetNames builds the exact asset names that are considered a direct match.
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

// scoreAssetMatch scores how well an asset name matches the current executable and platform.
func scoreAssetMatch(name string, execBases []string, goos, goarch string) assetMatch {
	baseMatched := assetMatchesAnyBase(name, execBases)
	osMatched, osMentioned := dimensionMatch(name, osAliases(goos), allOSAliases())
	archMatched, archMentioned := dimensionMatch(name, archAliases(goarch), allArchAliases())
	// An explicit reference to a different OS or architecture is treated as a hard
	// conflict so a clearly wrong asset never beats a generic one.
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

// filterByBaseMatch keeps only assets whose names match one of the executable base candidates.
func filterByBaseMatch(assets []githubReleaseAsset, execBases []string) []githubReleaseAsset {
	out := make([]githubReleaseAsset, 0, len(assets))
	for _, asset := range assets {
		if assetMatchesAnyBase(asset.Name, execBases) {
			out = append(out, asset)
		}
	}
	return out
}

// assetMatchesAnyBase reports whether an asset name appears to belong to any executable base candidate.
func assetMatchesAnyBase(name string, execBases []string) bool {
	assetBase := normalizeExecutableBase(name)
	for _, base := range execBases {
		if assetBase == base || strings.HasPrefix(assetBase, base+"-") || strings.HasPrefix(assetBase, base+"_") || strings.HasPrefix(assetBase, base+".") {
			return true
		}
	}
	return false
}

// isMetadataAsset reports whether an asset is checksum metadata rather than an installable binary.
func isMetadataAsset(nameLower string) bool {
	return nameLower == "checksums.txt" || isChecksumSignatureAsset(nameLower)
}

// isChecksumSignatureAsset reports whether an asset name looks like a detached checksum signature.
func isChecksumSignatureAsset(nameLower string) bool {
	switch nameLower {
	case "checksums.txt.sig", "checksums.sig", "checksums.txt.asc", "checksums.asc":
		return true
	default:
		return false
	}
}

// isArchiveAsset reports whether an asset name is an archive that this updater does not unpack.
func isArchiveAsset(nameLower string) bool {
	archiveSuffixes := []string{".zip", ".tar", ".tar.gz", ".tgz", ".tar.xz", ".txz", ".tar.bz2", ".tbz2", ".gz", ".xz", ".bz2"}
	for _, suffix := range archiveSuffixes {
		if strings.HasSuffix(nameLower, suffix) {
			return true
		}
	}
	return false
}

// dimensionMatch reports whether a name mentions the current dimension and whether it mentions any known dimension.
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

// containsPlatformToken reports whether a name contains alias as a standalone platform token.
func containsPlatformToken(name, alias string) bool {
	pattern := fmt.Sprintf(`(^|[^a-z0-9])%s([^a-z0-9]|$)`, regexp.QuoteMeta(strings.ToLower(alias)))
	return regexp.MustCompile(pattern).MatchString(strings.ToLower(name))
}

// osAliases returns the known release-asset aliases for a GOOS value.
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

// archAliases returns the known release-asset aliases for a GOARCH value.
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

// allOSAliases returns the known OS aliases indexed by canonical GOOS.
func allOSAliases() map[string][]string {
	return map[string][]string{
		"darwin":  osAliases("darwin"),
		"windows": osAliases("windows"),
		"linux":   osAliases("linux"),
	}
}

// allArchAliases returns the known architecture aliases indexed by canonical GOARCH.
func allArchAliases() map[string][]string {
	return map[string][]string{
		"amd64": archAliases("amd64"),
		"arm64": archAliases("arm64"),
		"386":   archAliases("386"),
		"arm":   archAliases("arm"),
	}
}

// uniqueStrings returns the distinct non-empty strings from values in sorted order.
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
