package semver

import (
	"errors"
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
	for i := 0; i < len(v.Pre) && i < len(o.Pre); i++ {
		va := v.Pre[i]
		ob := o.Pre[i]
		vaNum, vaErr := strconv.Atoi(va)
		obNum, obErr := strconv.Atoi(ob)
		if vaErr == nil && obErr == nil {
			if vaNum != obNum {
				return vaNum > obNum
			}
			// equal numeric, continue
		}
		if vaErr == nil && obErr != nil {
			// numeric < non-numeric
			return false
		}
		if vaErr != nil && obErr == nil {
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
	if len(parts) < 3 {
		return Version{}, fmt.Errorf("invalid semver (need major.minor.patch): %s", orig)
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, errors.New("invalid major version")
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, errors.New("invalid minor version")
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, errors.New("invalid patch version")
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
				// form: sig.<hex> (next part is hex)
				if i+1 < len(parts) {
					hex := parts[i+1]
					if isHex(hex) {
						v.Signature = &Signature{Algo: "sha256", Hex: hex}
						break
					}
				}
			}
			// also allow sig:<rest> or sig-...? Keep to dot-form per semver.
			if strings.HasPrefix(p, "sig") {
				// p could be "sig" (handled) or "sig<sep>..." but we only support
				// exact "sig" token followed by parts for clarity.
			}
			// handle sig.<algo>.<hex>
			if p == "sig" && i+2 < len(parts) {
				algo := strings.ToLower(parts[i+1])
				hex := parts[i+2]
				if isHex(hex) {
					v.Signature = &Signature{Algo: algo, Hex: hex}
					break
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
