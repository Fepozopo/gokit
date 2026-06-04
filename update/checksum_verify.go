package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fepozopo/gokit/osutil"
)

// parseTrustedPublicKeys decodes and validates the trusted ed25519 public keys
// used for checksum signature verification.
//
// The update package treats malformed trust material as a caller/configuration
// error, so this function fails fast instead of silently skipping bad entries.
// That makes release-verification failures easier to diagnose because the user
// sees which configured key is invalid before any network work is attempted.
func parseTrustedPublicKeys(trustedPubKeysHex []string) ([]ed25519.PublicKey, error) {
	if len(trustedPubKeysHex) == 0 {
		return nil, fmt.Errorf("no trusted public keys provided")
	}

	keys := make([]ed25519.PublicKey, 0, len(trustedPubKeysHex))
	for i, encodedKey := range trustedPubKeysHex {
		keyHex := strings.TrimSpace(encodedKey)
		if keyHex == "" {
			return nil, fmt.Errorf("trusted public key %d is empty", i+1)
		}

		decodedKey, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("trusted public key %d is not valid hex: %w", i+1, err)
		}
		if len(decodedKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("trusted public key %d has length %d bytes; want %d", i+1, len(decodedKey), ed25519.PublicKeySize)
		}
		keys = append(keys, ed25519.PublicKey(decodedKey))
	}
	return keys, nil
}

// verifyChecksumsSignature verifies the detached checksum signature against the
// raw checksums payload using the supplied trusted public keys.
//
// The public keys passed here must already be decoded and validated. Separating
// key parsing from signature verification keeps configuration errors distinct
// from genuine signature failures.
func verifyChecksumsSignature(ck []byte, sigHex string, trustedPubKeys []ed25519.PublicKey) error {
	sig, err := hex.DecodeString(strings.TrimSpace(sigHex))
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("unexpected signature length: %d", len(sig))
	}

	for _, trustedPubKey := range trustedPubKeys {
		if ed25519.Verify(trustedPubKey, ck, sig) {
			return nil
		}
	}
	return fmt.Errorf("signature did not verify against any trusted public key")
}

// parseChecksums parses and validates a checksums.txt payload.
//
// Each non-empty, non-comment line must contain exactly one SHA-256 checksum
// and one file name. The parser rejects malformed lines, invalid checksum
// lengths or hex, and duplicate file entries so callers get precise errors when
// release metadata is broken.
func parseChecksums(ck []byte) (map[string]string, error) {
	out := make(map[string]string)
	lines := strings.Split(string(ck), "\n")
	for i, line := range lines {
		lineNumber := i + 1
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		hash, name, err := parseChecksumLine(trimmedLine)
		if err != nil {
			return nil, fmt.Errorf("invalid checksums.txt line %d: %w", lineNumber, err)
		}
		if _, exists := out[name]; exists {
			return nil, fmt.Errorf("duplicate checksum entry for %q on line %d", name, lineNumber)
		}
		out[name] = hash
	}
	return out, nil
}

// parseChecksumLine parses a single checksums.txt data line into a normalized
// SHA-256 hash and file name.
//
// The parser accepts the common "hash<two spaces>file" format and also falls
// back to generic whitespace splitting for checksum tools that emit a single
// space. A leading '*' on the file name is stripped so binary-mode output from
// checksum utilities still matches the asset name used by the updater.
func parseChecksumLine(line string) (hash string, name string, err error) {
	var parts []string
	if strings.Contains(line, "  ") {
		parts = strings.SplitN(line, "  ", 2)
	} else {
		parts = strings.Fields(line)
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected one checksum and one file name")
	}

	normalizedHash, err := normalizeSHA256Hex(parts[0])
	if err != nil {
		return "", "", err
	}

	name = strings.TrimSpace(parts[1])
	name = strings.TrimPrefix(name, "*")
	if name == "" {
		return "", "", fmt.Errorf("missing file name")
	}
	return normalizedHash, name, nil
}

// normalizeSHA256Hex validates a SHA-256 checksum string and returns it in
// lowercase hexadecimal form.
//
// The updater verifies SHA-256 checksums specifically, so hashes with the wrong
// decoded length are rejected even if they are otherwise valid hexadecimal.
func normalizeSHA256Hex(hash string) (string, error) {
	normalizedHash := strings.TrimSpace(hash)
	if normalizedHash == "" {
		return "", fmt.Errorf("missing checksum")
	}

	decodedHash, err := hex.DecodeString(normalizedHash)
	if err != nil {
		return "", fmt.Errorf("checksum %q is not valid hex: %w", normalizedHash, err)
	}
	if len(decodedHash) != sha256.Size {
		return "", fmt.Errorf("checksum %q decodes to %d bytes; want %d", normalizedHash, len(decodedHash), sha256.Size)
	}
	return strings.ToLower(normalizedHash), nil
}

// downloadAndReplace downloads an asset to a sibling temp file and atomically
// replaces destPath.
//
// The download uses the updater's HTTP client and optional GitHub token auth so
// tests can supply a custom client and production code can share one client
// across the entire update workflow.
func (u updater) downloadAndReplace(assetURL, destPath string, verify bool, expectedHex string) error {
	resp, err := doGetWithGitHubToken(u.httpClient, assetURL, nil)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer closeResponseBody(resp, assetURL)

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned status %d: %s", resp.StatusCode, string(b))
	}

	dir := filepath.Dir(destPath)
	tmpFile, err := os.CreateTemp(dir, ".tmp-upd-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	var shaSum []byte
	if verify {
		h := sha256.New()
		// Hash while streaming into the temp file so the verified bytes are exactly
		// the bytes that would be installed if replacement succeeds.
		if _, err := io.Copy(io.MultiWriter(tmpFile, h), resp.Body); err != nil {
			return fmt.Errorf("write temp: %w", err)
		}
		shaSum = h.Sum(nil)
	} else {
		if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			return fmt.Errorf("write temp: %w", err)
		}
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	if verify {
		got := fmt.Sprintf("%x", shaSum)
		if !strings.EqualFold(got, strings.TrimSpace(expectedHex)) {
			return fmt.Errorf("checksum mismatch: expected %s got %s", expectedHex, got)
		}
	}

	if err := osutil.AtomicReplace(tmpName, destPath); err != nil {
		return fmt.Errorf("replace failed: %w", err)
	}
	return nil
}
