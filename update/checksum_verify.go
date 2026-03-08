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
)

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

// downloadToTemp downloads url to a temp file in dir and returns the path.
func downloadToTemp(url, dir, prefix string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}
	tmpf, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer tmpf.Close()
	if _, err := io.Copy(tmpf, resp.Body); err != nil {
		os.Remove(tmpf.Name())
		return "", fmt.Errorf("write temp: %w", err)
	}
	return tmpf.Name(), nil
}

// downloadAndVerifyAndReplace downloads assetURL, verifies its SHA256 matches
// expectedHex, and atomically replaces destPath with the downloaded file.
// It uses the same atomic replacement strategy as the existing updater.
func downloadAndVerifyAndReplace(assetURL, expectedHex, destPath string) error {
	resp, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned status %d: %s", resp.StatusCode, string(b))
	}

	dir := filepath.Dir(destPath)
	tmpFile, err := os.CreateTemp(dir, ".timp-upd-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmpFile.Name()
	// ensure cleanup on failure
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	got := fmt.Sprintf("%x", hasher.Sum(nil))
	if strings.ToLower(got) != strings.ToLower(strings.TrimSpace(expectedHex)) {
		return fmt.Errorf("checksum mismatch: expected %s got %s", expectedHex, got)
	}

	// Preserve mode if destination exists; otherwise ensure executable bit for user
	if fi, err := os.Stat(destPath); err == nil {
		_ = os.Chmod(tmpName, fi.Mode())
	} else {
		_ = os.Chmod(tmpName, 0755)
	}

	// Attempt atomic rename
	if err := os.Rename(tmpName, destPath); err != nil {
		// cross-device fallback
		if errorsIs(err, "EXDEV") {
			if cerr := copyFile(tmpName, destPath); cerr != nil {
				return fmt.Errorf("copy fallback failed: %w (rename err: %v)", cerr, err)
			}
			_ = os.Remove(tmpName)
		} else if runtimeGOOS() == "windows" {
			_ = os.Remove(destPath)
			if rerr := os.Rename(tmpName, destPath); rerr != nil {
				return fmt.Errorf("rename after remove failed: %w", rerr)
			}
		} else {
			return fmt.Errorf("rename failed: %w", err)
		}
	}

	// fsync containing directory (best-effort)
	if dirf, err := os.Open(dir); err == nil {
		_ = dirf.Sync()
		_ = dirf.Close()
	}

	return nil
}

// These small wrappers avoid importing syscall/runtime in this file so tests
// can more easily stub if needed.
func errorsIs(err error, s string) bool {
	if err == nil {
		return false
	}
	// very small portability shim: check string contains
	return strings.Contains(err.Error(), s)
}

func runtimeGOOS() string { return os.Getenv("GOOS_OVERRIDE") }
