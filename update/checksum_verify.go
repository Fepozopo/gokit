package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fepozopo/gokit/osutil"
)

// defaultHTTPClient is used by helper download functions to ensure timeouts.
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// verifyChecksumsSignature verifies the hex-encoded signature sigHex over the
// checksums bytes ck using any of the trustedPubKeys (hex decoded). Returns
// nil on success.
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

// parseChecksums parses a checksums.txt file in format "<hex><two spaces><filename>\n"
// and returns a map filename->hex. Lines starting with '#' or empty lines are ignored.
func parseChecksums(ck []byte) map[string]string {
	out := make(map[string]string)
	lines := strings.Split(string(ck), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		// allow either two spaces or a single space separator
		var parts []string
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

// downloadAndReplace downloads assetURL to a temporary file in the same directory
// as destPath and then atomically replaces destPath with the downloaded file.
// If verify is true, it computes the SHA256 of the download and compares it to
// expectedHex before performing the replacement.
func downloadAndReplace(assetURL, destPath string, verify bool, expectedHex string) error {
	resp, err := defaultHTTPClient.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("download response body close failed", "url", assetURL, "err", cerr)
		}
	}()
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
	// ensure cleanup on failure
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	// Stream download into temp file; optionally compute hash while streaming.
	var hasher io.Writer
	var shaSum []byte
	if verify {
		h := sha256.New()
		hasher = h
		// copy into both temp file and hasher
		if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body); err != nil {
			return fmt.Errorf("write temp: %w", err)
		}
		shaSum = h.Sum(nil)
	} else {
		// just write to temp file
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

	// If verification requested, compare computed hash with expected.
	if verify {
		got := fmt.Sprintf("%x", shaSum)
		if !strings.EqualFold(got, strings.TrimSpace(expectedHex)) {
			return fmt.Errorf("checksum mismatch: expected %s got %s", expectedHex, got)
		}
	}

	// Atomically replace destPath with the temp file.
	if err := osutil.AtomicReplace(tmpName, destPath); err != nil {
		return fmt.Errorf("replace failed: %w", err)
	}

	return nil
}
