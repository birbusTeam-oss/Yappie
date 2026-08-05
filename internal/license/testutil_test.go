package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"time"
)

// testPrivateKey is loaded from the embedded private key PEM for test key generation.
// Only used in test files.
var testPrivateKey ed25519.PrivateKey

// loadTestPrivateKey loads the private key from disk for test key generation.
// Call this from TestMain or at the start of tests that need to generate keys.
func loadTestPrivateKey() error {
	if testPrivateKey != nil {
		return nil
	}
	data, err := os.ReadFile("license_private.pem")
	if err != nil {
		return fmt.Errorf("cannot read test private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("invalid private key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("cannot parse private key: %w", err)
	}
	testPrivateKey = key.(ed25519.PrivateKey)
	return nil
}

// GenerateTestKey creates a signed license key for testing.
// Uses the private key from license_private.pem.
func GenerateTestKey(email, plan string, expiry time.Time, machineID string, features []string) (string, error) {
	if err := loadTestPrivateKey(); err != nil {
		return "", err
	}

	payload := LicensePayload{
		Email:      email,
		Plan:       plan,
		ExpiryDate: expiry,
		MachineID:  machineID,
		IssuedAt:   time.Now(),
		Features:   features,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	sig := ed25519.Sign(testPrivateKey, payloadBytes)

	key := base64.RawURLEncoding.EncodeToString(payloadBytes) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
	return key, nil
}