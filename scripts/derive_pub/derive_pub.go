package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	var b64 string
	if len(os.Args) >= 2 && strings.TrimSpace(os.Args[1]) != "-" {
		b64 = os.Args[1]
	} else {
		// read from stdin
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed reading stdin:", err)
			os.Exit(2)
		}
		b64 = strings.TrimSpace(string(b))
	}
	seed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid base64 seed:", err)
		os.Exit(2)
	}
	if len(seed) != ed25519.SeedSize {
		fmt.Fprintln(os.Stderr, "seed must be 32 bytes (base64-encoded)")
		os.Exit(2)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	fmt.Println(hex.EncodeToString(pub))
}
