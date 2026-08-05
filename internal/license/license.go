package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "embed"
)

//go:embed license_pubkey.pem
var licensePubkeyPem []byte

var licensePubkey ed25519.PublicKey

func init() {
	block, _ := pem.Decode(licensePubkeyPem)
	if block == nil {
		panic("invalid license public key PEM: no block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic("invalid license public key: " + err.Error())
	}
	licensePubkey = pub.(ed25519.PublicKey)
}

// LicensePayload is the signed payload embedded in a license key.
type LicensePayload struct {
	Email      string    `json:"email"`
	Plan       string    `json:"plan"`       // "pro" | "lifetime"
	ExpiryDate time.Time `json:"expiry"`     // zero value = no expiry (lifetime)
	MachineID  string    `json:"machine_id"` // empty = not machine-bound
	IssuedAt   time.Time `json:"issued_at"`
	Features   []string  `json:"features"`
}

// License represents a validated license key
type License struct {
	Key      string    `json:"key"`
	Plan     string    `json:"plan"` // "free", "pro", "lifetime"
	Features []string  `json:"features"`
	Valid    bool      `json:"valid"`
	Checked  time.Time `json:"checked"`
}

// Manager handles license validation and caching
type Manager struct {
	mu      sync.Mutex
	license *License
	path    string
}

var (
	instance *Manager
	once     sync.Once
)

// GetManager returns the singleton license manager
func GetManager() *Manager {
	once.Do(func() {
		dir := filepath.Join(os.Getenv("APPDATA"), "Yappie")
		os.MkdirAll(dir, 0755)
		instance = &Manager{
			path: filepath.Join(dir, "license.json"),
		}
		instance.load()
	})
	return instance
}

// NewManager creates a Manager with a custom path (for testing)
func NewManager(path string) *Manager {
	m := &Manager{path: path}
	m.load()
	return m
}

// IsPro returns true if the current license is Pro or Lifetime
func (m *Manager) IsPro() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.license != nil && m.license.Valid && (m.license.Plan == "pro" || m.license.Plan == "lifetime")
}

// IsLifetime returns true if the current license is Lifetime
func (m *Manager) IsLifetime() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.license != nil && m.license.Valid && m.license.Plan == "lifetime"
}

// HasFeature checks if a specific Pro feature is unlocked
func (m *Manager) HasFeature(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.license == nil || !m.license.Valid {
		return false
	}
	for _, f := range m.license.Features {
		if f == name {
			return true
		}
	}
	return false
}

// GetLicense returns the current license info
func (m *Manager) GetLicense() *License {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.license
}

// ValidateKey verifies the Ed25519 signature on the license key,
// checks expiry and machine binding. Works fully offline — no network calls.
func (m *Manager) ValidateKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("empty license key")
	}

	// Key format: base64url(payload) + "." + base64url(signature)
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid license key format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("invalid payload encoding: %w", err)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !ed25519.Verify(licensePubkey, payloadBytes, sigBytes) {
		return fmt.Errorf("license signature verification failed")
	}

	var payload LicensePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("invalid license data: %w", err)
	}

	// Check expiry (zero time = lifetime, no expiry check)
	if !payload.ExpiryDate.IsZero() && time.Now().After(payload.ExpiryDate) {
		return fmt.Errorf("license expired on %s", payload.ExpiryDate.Format("2006-01-02"))
	}

	// Check machine binding (empty = no binding)
	if payload.MachineID != "" {
		mid, err := machineID()
		if err != nil {
			return fmt.Errorf("cannot determine machine ID: %w", err)
		}
		if payload.MachineID != mid {
			return fmt.Errorf("license not valid for this machine")
		}
	}

	m.mu.Lock()
	m.license = &License{
		Key:      key,
		Plan:     payload.Plan,
		Features: payload.Features,
		Valid:    true,
		Checked:  time.Now(),
	}
	m.mu.Unlock()

	return m.save()
}

// ClearLicense removes the stored license
func (m *Manager) ClearLicense() error {
	m.mu.Lock()
	m.license = nil
	m.mu.Unlock()
	if m.path == "" {
		return nil
	}
	return os.Remove(m.path)
}

func (m *Manager) load() {
	if m.path == "" {
		return
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		return
	}
	var lic License
	if err := json.Unmarshal(data, &lic); err != nil {
		return
	}
	m.mu.Lock()
	m.license = &lic
	m.mu.Unlock()
}

func (m *Manager) save() error {
	if m.path == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.license == nil {
		return nil
	}
	data, err := json.MarshalIndent(m.license, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0644)
}

// machineID returns a machine-specific identifier. On non-Windows or if
// the machineid package is unavailable, returns a fallback based on hostname.
func machineID() (string, error) {
	// In production, use github.com/denisbrodbeck/machineid
	// For now, use hostname as a cross-platform fallback
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}