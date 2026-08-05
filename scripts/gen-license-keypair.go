//go:build ignore

// Generate Ed25519 keypair for license signing.
// Run: go run scripts/gen-license-keypair.go
// Outputs: license_private.pem (keep secret), license_pubkey.pem (embed in app)

package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func main() {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		panic(err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic(err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	// Write private key
	if err := os.WriteFile("internal/license/license_private.pem", privPEM, 0600); err != nil {
		panic(err)
	}
	fmt.Println("Private key written to internal/license/license_private.pem (DO NOT COMMIT)")

	// Write public key
	if err := os.WriteFile("internal/license/license_pubkey.pem", pubPEM, 0644); err != nil {
		panic(err)
	}
	fmt.Println("Public key written to internal/license/license_pubkey.pem (safe to commit)")

	// Also print raw hex for debugging
	fmt.Printf("\nPublic key hex:  %x\n", pub)
	fmt.Printf("Private key hex: %x\n", priv)
}