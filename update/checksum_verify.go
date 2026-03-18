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
	"runtime"
	"strings"
	"time"
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
	defer resp.Body.Close()
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

// copyFile copies a file from src to dst, preserving permissions.
// Used as a fallback when atomic rename is not possible (e.g. cross-device).
// Returns an error on failure.
func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src failed: %w", err)
	}
	defer sf.Close()
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat src failed: %w", err)
	}
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode())
	if err != nil {
		return fmt.Errorf("open dst failed: %w", err)
	}
	defer df.Close()
	if _, err := io.Copy(df, sf); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	if err := df.Sync(); err != nil {
		return fmt.Errorf("sync dst failed: %w", err)
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

func runtimeGOOS() string {
	if v := os.Getenv("GOOS_OVERRIDE"); v != "" {
		return v
	}
	return runtime.GOOS
}
