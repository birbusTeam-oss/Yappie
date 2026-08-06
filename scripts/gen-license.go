//go:build ignore

// gen-license.go — Generate signed Ed25519 license keys for Yappie Pro.
//
// Usage:
//   go run scripts/gen-license.go -email user@example.com -plan pro|lifetime [-expiry 2026-12-31]
//
// Output: A signed license key printed to stdout in the format:
//   base64url(payloadJSON).base64url(ed25519Signature)
//
// The private key is loaded from internal/license/license_private.pem.

package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LicensePayload mirrors the struct in internal/license/license.go.
// Duplicated here so this script has no dependency on the main module.
type LicensePayload struct {
	Email      string    `json:"email"`
	Plan       string    `json:"plan"`
	ExpiryDate time.Time `json:"expiry"`
	MachineID  string    `json:"machine_id"`
	IssuedAt   time.Time `json:"issued_at"`
	Features   []string  `json:"features"`
}

var proFeatures = []string{
	"multi_language",
	"custom_vocab",
	"snippets",
	"cloud_sync",
	"custom_hotkeys",
}

var lifetimeFeatures = []string{
	"multi_language",
	"custom_vocab",
	"snippets",
	"cloud_sync",
	"custom_hotkeys",
	"priority_support",
}

func main() {
	email := flag.String("email", "", "Customer email (required)")
	plan := flag.String("plan", "", "License plan: pro or lifetime (required)")
	expiryStr := flag.String("expiry", "", "Custom expiry date (YYYY-MM-DD). Defaults to 30 days for pro, no expiry for lifetime")
	flag.Parse()

	if *email == "" || *plan == "" {
		fmt.Fprintln(os.Stderr, "Usage: go run scripts/gen-license.go -email user@example.com -plan pro|lifetime [-expiry 2026-12-31]")
		os.Exit(1)
	}

	if *plan != "pro" && *plan != "lifetime" {
		fmt.Fprintf(os.Stderr, "Error: plan must be 'pro' or 'lifetime', got '%s'\n", *plan)
		os.Exit(1)
	}

	// Determine features and expiry
	var features []string
	var expiry time.Time

	switch *plan {
	case "pro":
		features = proFeatures
		if *expiryStr != "" {
			t, err := time.Parse("2006-01-02", *expiryStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing expiry date: %v\n", err)
				os.Exit(1)
			}
			expiry = t
		} else {
			expiry = time.Now().Add(30 * 24 * time.Hour) // 30 days from now
		}
	case "lifetime":
		features = lifetimeFeatures
		// Lifetime = zero time (no expiry). Ignore -expiry flag.
		expiry = time.Time{}
	}

	// Load private key
	keyPath := filepath.Join("internal", "license", "license_private.pem")
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading private key (%s): %v\n", keyPath, err)
		fmt.Fprintln(os.Stderr, "Tip: run this from the quill-go project root directory")
		os.Exit(1)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		fmt.Fprintln(os.Stderr, "Error: invalid private key PEM — no block found")
		os.Exit(1)
	}

	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 / raw formats as fallback
		privKey2, err2 := x509.ParseECPrivateKey(block.Bytes)
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "Error parsing private key: %v\n", err)
			os.Exit(1)
		}
		privKey = privKey2
	}

	ed25519Priv, ok := privKey.(ed25519.PrivateKey)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: key is not Ed25519 (got %T)\n", privKey)
		os.Exit(1)
	}

	// Build payload
	payload := LicensePayload{
		Email:      *email,
		Plan:       *plan,
		ExpiryDate: expiry,
		MachineID:  "", // not machine-bound by default
		IssuedAt:   time.Now().UTC(),
		Features:   features,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling payload: %v\n", err)
		os.Exit(1)
	}

	// Sign
	signature := ed25519.Sign(ed25519Priv, payloadJSON)

	// Encode as base64url(payload).base64url(signature)
	keyStr := base64.RawURLEncoding.EncodeToString(payloadJSON) + "." +
		base64.RawURLEncoding.EncodeToString(signature)

	// Print to stdout
	fmt.Println(keyStr)

	// Print metadata to stderr for the operator
	fmt.Fprintf(os.Stderr, "\nLicense generated:\n")
	fmt.Fprintf(os.Stderr, "  Email:  %s\n", *email)
	fmt.Fprintf(os.Stderr, "  Plan:   %s\n", *plan)
	if expiry.IsZero() {
		fmt.Fprintf(os.Stderr, "  Expiry: never (lifetime)\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Expiry: %s\n", expiry.Format("2006-01-02"))
	}
	fmt.Fprintf(os.Stderr, "  Features: %v\n", features)
	fmt.Fprintf(os.Stderr, "\nCopy the key above (first line only) and send to the customer.\n")
}