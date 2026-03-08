package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
)

func loadSeedFromFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed reading seed file: %w", err)
	}
	// If the file is exactly 32 bytes, assume it's the raw seed.
	if len(data) == 32 {
		return data, nil
	}
	// Otherwise try to decode as base64 text (trim whitespace/newlines).
	s := strings.TrimSpace(string(data))
	seed, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("seed file is not 32 bytes raw nor valid base64: %w", err)
	}
	return seed, nil
}

func main() {
	// Usage: sign_checksums <checksums.txt> <seed_file>
	if len(os.Args) < 3 {
		log.Fatalf("usage: %s <checksums.txt> <seed_file>\nseed_file may be a raw 32-byte file or base64-encoded text.", os.Args[0])
	}
	ckFile := os.Args[1]
	seedFile := os.Args[2]

	seed, err := loadSeedFromFile(seedFile)
	if err != nil {
		log.Fatalf("error loading seed from file %s: %v", seedFile, err)
	}

	if len(seed) != 32 {
		log.Fatalf("seed must be 32 bytes (raw seed)")
	}
	priv := ed25519.NewKeyFromSeed(seed)

	ck, err := os.ReadFile(ckFile)
	if err != nil {
		log.Fatalf("failed reading checksums file: %v", err)
	}
	sig := ed25519.Sign(priv, ck)
	sigHex := hex.EncodeToString(sig)
	if err := os.WriteFile(ckFile+".sig", []byte(sigHex), 0644); err != nil {
		log.Fatalf("failed writing signature file: %v", err)
	}
	log.Printf("signed %s -> %s.sig", ckFile, ckFile)
}
