# Scripts

This repository is a library repo. The scripts in this directory are optional
helpers intended to be **copied or adapted into downstream application repos**
that ship binaries.

They are not required to use any package in `gokit`, and they are not this
repository's own release pipeline.

## Included helpers

- `build-all.sh` — cross-compile one or more explicitly provided Go `main`
  packages, generate `checksums.txt`, and optionally sign it.
- `generate_seed.sh` — create a raw 32-byte ed25519 seed file.
- `generate_pubkey.sh` — derive and print the hex public key for a seed.
- `derive_pub/derive_pub.go` — Go helper used to derive a public key from a
  base64-encoded seed.
- `sign_checksums/sign_checksums.go` — sign a `checksums.txt` file with an
  ed25519 seed.

## Typical downstream usage

From an application repo that contains `main` packages:

```bash
./scripts/build-all.sh -o ./dist -t v1.2.3 ./cmd/myapp ./cmd/worker
```

If `./ed25519_seed.bin` exists, `build-all.sh` will also write
`checksums.txt.sig`.

## Notes

- `build-all.sh` expects explicit package paths; it does not scan `./cmd/*`.
- The helper uses `go build` directly, so the target packages must be buildable
  `main` packages.
- Keep ed25519 seed material private and out of version control.
