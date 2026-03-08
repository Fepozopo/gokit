#!/usr/bin/env bash
# after generating ed25519_seed.bin (32 bytes)
SEED_B64=$(base64 < ed25519_seed.bin)
go run scripts//derive_pub/derive_pub.go "$SEED_B64" # prints 64-hex pubkey
