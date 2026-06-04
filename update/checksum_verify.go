package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fepozopo/gokit/osutil"
)

// verifyChecksumsSignature verifies the detached checksum signature against the raw checksums file.
func verifyChecksumsSignature(ck []byte, sigHex string, trustedPubKeysHex []string) error {
	sig, err := hex.DecodeString(strings.TrimSpace(sigHex))
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("unexpected signature length: %d", len(sig))
	}
	for _, ph := range trustedPubKeysHex {
		pbh := strings.TrimSpace(ph)
		pub, err := hex.DecodeString(pbh)
		if err != nil {
			continue
		}
		if len(pub) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(pub), ck, sig) {
			return nil
		}
	}
	return fmt.Errorf("checksums signature verification failed")
}

// parseChecksums parses a checksums.txt file into a filename-to-hash map.
func parseChecksums(ck []byte) map[string]string {
	out := make(map[string]string)
	lines := strings.Split(string(ck), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		var parts []string
		// Support the common "hash<two spaces>file" format while remaining tolerant
		// of checksum files that separate fields with generic whitespace.
		if strings.Contains(l, "  ") {
			parts = strings.SplitN(l, "  ", 2)
		} else {
			parts = strings.Fields(l)
		}
		if len(parts) < 2 {
			continue
		}
		hash := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		out[name] = strings.ToLower(hash)
	}
	return out
}

// downloadAndReplace downloads an asset to a sibling temp file and atomically replaces destPath.
func downloadAndReplace(assetURL, destPath string, verify bool, expectedHex string) error {
	resp, err := doGetWithGitHubToken(defaultHTTPClient, assetURL, nil)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer closeResponseBody(resp, assetURL)

	if resp.StatusCode != 200 {
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
