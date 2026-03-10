# gokit

Small collection of reusable Go helpers and release utilities used across projects.

This repository contains a few focused packages and helper scripts that make it
easy to parse/compare semantic versions, implement a secure self-update flow
(signed checksums + ed25519 verification), and load simple `.env` files.

Contents

- `semver/` — parse and compare semantic versions. Supports pre-release, build
  metadata and parsing a lightweight signature token from build metadata.
- `update/` — helpers to detect releases on GitHub, verify signed `checksums.txt`,
  download artifacts and atomically replace the running executable.
- `utils/` — small utilities; currently `LoadDotEnv` for simple `.env` parsing.
- `scripts/` — build and signing helpers (`build-all.sh`, signing and key derivation tools).

Quick examples

Parse a version and read a parsed signature (if present):

```go
package main

import (
    "fmt"
    "log"

    "github.com/Fepozopo/gokit/semver"
)

func main() {
    v, err := semver.Parse("v1.2.3+sig.sha256.deadbeef")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("version:", v.String())
    if v.HasSignature() {
        fmt.Printf("signature: algo=%s hex=%s\n", v.Signature.Algo, v.Signature.Hex)
    }
}
```

Check for updates and apply them (basic pattern):

```go
package main

import (
    "log"

    "github.com/Fepozopo/gokit/update"
)

func main() {
    currentVersion := "v0.1.0" // replace with actual version (set at build time)
    repo := "owner/repo"       // replace with actual repo

    // example hex public key(s) for signature verification; keep the seed private
    trustedPubKeysHex := []string{"a3f1c2d4e5b6a7980f1e2d3c4b5a6978c9d0e1f2a3b4c5d6e7f8091a2b3c4d5e"}

    res, err := update.CheckForUpdates(currentVersion, repo)
    if err != nil {
        log.Fatal(err)
    }

    if !res.Available {
        // The Err field contains sentinel errors callers may inspect,
        // e.g. update.ErrNoReleases, update.ErrNoAsset, update.ErrMissingChecksums
        if res.Err != nil {
            log.Printf("no update available: %v", res.Err)
        } else {
            log.Println("already up-to-date")
        }
        return
    }

    // When `verify` is true the update will verify checksums.txt with the provided trusted public keys (recommended).
    if err := update.Update(repo, res.Latest, true, trustedPubKeysHex); err != nil {
        log.Fatalf("update failed: %v", err)
    }
}
```

Load a `.env` file into environment variables:

```go
import "github.com/Fepozopo/gokit/utils"

if err := utils.LoadDotEnv(".env"); err != nil {
    // handle error or ignore (LoadDotEnv returns an error if file can't be read)
}
```

Build & release workflow

1. Generate an ed25519 seed (keep it private):

```bash
openssl rand -out ed25519_seed.bin 32
```

2. Derive the hex public key (do not commit the seed):

```bash
SEED_B64=$(base64 < ed25519_seed.bin)
go run scripts/derive_pub/derive_pub.go "$SEED_B64"
# prints 64-hex public key
```

3. Provide the public key(s) to your application (in code, configuration, or environment) so it can verify release signatures. For example, embed the hex public key and pass it to `update.Update(...)`, or load trusted keys at runtime from a secure config.

Notes on update checking

- `CheckForUpdates` returns an `UpdateCheckResult` with fields `Available`, `Latest`, and `Err`. Inspect `UpdateCheckResult.Err` for programmatic hints (sentinel errors exported from the `update` package): `ErrNoReleases`, `ErrNoAsset`, `ErrMissingChecksums`, and `ErrCurrentVersionInvalid`.

4. Build artifacts and produce `checksums.txt` (script will sign it if `ed25519_seed.bin` exists):

```bash
./scripts/build-all.sh
```

5. Upload built binaries plus `checksums.txt` and `checksums.txt.sig` to a GitHub Release.

Notes

- `checksums.txt` entries use SHA256 in the form: `<sha256>  <filename>` (two spaces). The parser accepts single-space variants too.
- Keep `ed25519_seed.bin` secret; it is listed in `.gitignore` by default.
- Add one or more trusted public keys in to allow key rotation.

Testing

Run unit tests:

```bash
go test ./...
```

License

MIT. See `LICENSE` for details.
