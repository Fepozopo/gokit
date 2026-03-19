package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a semantic version (core + optional pre-release and build metadata).
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   []string // pre-release identifiers
	Build string   // build metadata
	// Signature, if present, is parsed out of Build when the version string
	// contains a canonical signature token (see Parse).
	Signature *Signature
}

// Signature holds signing metadata parsed from build metadata. It's intentionally
// lightweight: `Algo` is a string like "sha256" and `Hex` holds the hex-encoded
// signature/token. Presence of a non-nil Signature does not affect semver
// precedence or equality (build metadata is ignored by semver rules).
type Signature struct {
	Algo string
	Hex  string
}

func (v Version) String() string {
	core := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		core = core + "-" + strings.Join(v.Pre, ".")
	}
	if v.Build != "" {
		core = core + "+" + v.Build
	}
	return core
}

// HasSignature reports whether a signature was parsed from the build metadata.
func (v Version) HasSignature() bool {
	return v.Signature != nil && v.Signature.Hex != ""
}

// Equals returns true if versions are equal for update purposes (ignores build metadata).
func (v Version) Equals(o Version) bool {
	if v.Major != o.Major || v.Minor != o.Minor || v.Patch != o.Patch {
		return false
	}
	if len(v.Pre) != len(o.Pre) {
		return false
	}
	for i := range v.Pre {
		if v.Pre[i] != o.Pre[i] {
			return false
		}
	}
	return true
}

// GT returns true if v > o according to semver precedence rules.
func (v Version) GT(o Version) bool {
	if v.Major != o.Major {
		return v.Major > o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor > o.Minor
	}
	if v.Patch != o.Patch {
		return v.Patch > o.Patch
	}
	// Handle pre-release: absence of pre-release has higher precedence than presence.
	if len(v.Pre) == 0 && len(o.Pre) == 0 {
		return false // equal
	}
	if len(v.Pre) == 0 && len(o.Pre) > 0 {
		return true // v is release, o is pre-release => v > o
	}
	if len(v.Pre) > 0 && len(o.Pre) == 0 {
		return false // v is pre-release, o is release => v < o
	}
	// both have pre-release: compare identifier by identifier
	// Numeric identifiers are compared numerically. To avoid platform
	// dependent integer overflow we compare numeric identifiers by
	// length then lexicographically when lengths are equal (equivalent
	// to numeric comparison for no-leading-zero numeric strings).
	for i := 0; i < len(v.Pre) && i < len(o.Pre); i++ {
		va := v.Pre[i]
		ob := o.Pre[i]
		vaIsNum := isDigits(va)
		obIsNum := isDigits(ob)
		if vaIsNum && obIsNum {
			if len(va) != len(ob) {
				return len(va) > len(ob)
			}
			if va != ob {
				return va > ob
			}
			// equal numeric, continue
			continue
		}
		if vaIsNum && !obIsNum {
			// numeric < non-numeric
			return false
		}
		if !vaIsNum && obIsNum {
			return true
		}
		// both non-numeric, lexicographical compare
		if va != ob {
			return va > ob
		}
	}
	// all compared equal so far; longer pre-release has higher precedence
	return len(v.Pre) > len(o.Pre)
}

// Parse parses a semantic version string (allows optional leading 'v').
//   - major/minor/patch must be non-negative integers without leading zeros (except "0")
//   - pre-release identifiers must be ASCII alphanumerics or hyphen; if numeric, they must not have leading zeros
func Parse(s string) (Version, error) {
	orig := s
	if strings.HasPrefix(s, "v") || strings.HasPrefix(s, "V") {
		s = s[1:]
	}
	var build string
	if idx := strings.Index(s, "+"); idx >= 0 {
		build = s[idx+1:]
		s = s[:idx]
	}
	var pre []string
	if idx := strings.Index(s, "-"); idx >= 0 {
		pre = strings.Split(s[idx+1:], ".")
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid semver core (need major.minor.patch): %q", orig)
	}
	// validate core numeric identifiers: no leading zeros (unless "0"), no negatives, digits only
	for _, p := range parts[:3] {
		if !isDigits(p) {
			return Version{}, fmt.Errorf("invalid numeric version %q in %q", p, orig)
		}
		if !isNumericNoLeadingZeros(p) {
			return Version{}, fmt.Errorf("invalid numeric identifier (leading zero) %q in %q", p, orig)
		}
	}
	maj, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])

	// validate pre-release identifiers per semver (if any)
	if len(pre) > 0 {
		for _, ident := range pre {
			if ident == "" {
				return Version{}, fmt.Errorf("empty pre-release identifier in %q", orig)
			}
			if !isValidPrereleaseIdent(ident) {
				return Version{}, fmt.Errorf("invalid pre-release identifier %q in %q", ident, orig)
			}
			// if numeric, must not have leading zeros
			if isDigits(ident) && !isNumericNoLeadingZeros(ident) {
				return Version{}, fmt.Errorf("numeric pre-release identifier with leading zeros %q in %q", ident, orig)
			}
		}
	}

	v := Version{Major: maj, Minor: min, Patch: patch, Pre: pre, Build: build}

	// Parse signature token out of build metadata if present. Accept forms:
	//  - sig.<hex>            (assume algo sha256)
	//  - sig.<algo>.<hex>
	// The build metadata may contain multiple dot-separated identifiers; scan
	// them to find a segment starting with "sig".
	if build != "" {
		parts := strings.Split(build, ".")
		for i := 0; i < len(parts); i++ {
			p := strings.ToLower(parts[i])
			if p == "sig" {
				// form: sig.<hex> or sig.<algo>.<hex>
				if i+1 < len(parts) {
					if isHex(parts[i+1]) {
						v.Signature = &Signature{Algo: "sha256", Hex: parts[i+1]}
						break
					}
					if i+2 < len(parts) && isHex(parts[i+2]) {
						algo := strings.ToLower(parts[i+1])
						v.Signature = &Signature{Algo: algo, Hex: parts[i+2]}
						break
					}
				}
			}
		}
	}
	return v, nil
}

// isHex returns true if s contains only hex characters (0-9, a-f, A-F) and
// has at least one character.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return true
}

// isDigits returns true if s contains only ASCII digits and at least one char.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	// iterate bytes to ensure ASCII-only digits and avoid rune-width surprises
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// isNumericNoLeadingZeros returns true if s is "0" or does not start with '0'.
func isNumericNoLeadingZeros(s string) bool {
	if s == "" {
		return false
	}
	if s == "0" {
		return true
	}
	return !(len(s) > 1 && s[0] == '0')
}

// isValidPrereleaseIdent returns true if the identifier contains only
// ASCII alphanumerics and hyphen (per semver) and is non-empty.
func isValidPrereleaseIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		// restrict to ASCII letters/digits and hyphen
		if r == '-' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		return false
	}
	return true
}
