package license

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateKey(t *testing.T) {
	// Generate test keys using the private key
	futureExpiry := time.Now().Add(365 * 24 * time.Hour)
	pastExpiry := time.Now().Add(-24 * time.Hour)

	validProKey, err := GenerateTestKey(
		"test@example.com", "pro", futureExpiry, "",
		[]string{"multi_language", "custom_vocab", "snippets", "cloud_sync", "custom_hotkeys"},
	)
	if err != nil {
		t.Fatalf("GenerateTestKey failed: %v", err)
	}

	validLifetimeKey, err := GenerateTestKey(
		"test@example.com", "lifetime", time.Time{}, "",
		[]string{"multi_language", "custom_vocab", "snippets", "cloud_sync", "custom_hotkeys", "priority_support"},
	)
	if err != nil {
		t.Fatalf("GenerateTestKey failed: %v", err)
	}

	expiredKey, err := GenerateTestKey(
		"test@example.com", "pro", pastExpiry, "",
		[]string{"multi_language"},
	)
	if err != nil {
		t.Fatalf("GenerateTestKey failed: %v", err)
	}

	wrongMachineKey, err := GenerateTestKey(
		"test@example.com", "pro", futureExpiry, "wrong-machine-id",
		[]string{"multi_language"},
	)
	if err != nil {
		t.Fatalf("GenerateTestKey failed: %v", err)
	}

	// Create a tampered key by modifying the payload
	validProKeyBytes := []byte(validProKey)
	tamperedKey := string(validProKeyBytes[:10]) + "X" + string(validProKeyBytes[11:])

	tests := []struct {
		name    string
		key     string
		wantErr string
		wantPlan string
	}{
		{"Valid Pro key", validProKey, "", "pro"},
		{"Valid Lifetime key", validLifetimeKey, "", "lifetime"},
		{"Expired key", expiredKey, "license expired", ""},
		{"Wrong machine", wrongMachineKey, "not valid for this machine", ""},
		{"Tampered signature", tamperedKey, "license signature verification failed", ""},
		{"Empty key", "", "empty license key", ""},
		{"Malformed key - no dot", "garbage", "invalid license key format", ""},
		{"Malformed base64", "not.base64!!!", "invalid signature encoding", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			m := NewManager(filepath.Join(tmpDir, "license.json"))

			err := m.ValidateKey(tt.key)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
					return
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.wantPlan != "" {
				lic := m.GetLicense()
				if lic == nil {
					t.Fatal("expected license to be set, got nil")
				}
				if lic.Plan != tt.wantPlan {
					t.Errorf("expected plan %q, got %q", tt.wantPlan, lic.Plan)
				}
				if !lic.Valid {
					t.Error("expected license to be valid")
				}
			}
		})
	}
}

func TestIsPro(t *testing.T) {
	futureExpiry := time.Now().Add(365 * 24 * time.Hour)

	proKey, _ := GenerateTestKey("test@example.com", "pro", futureExpiry, "",
		[]string{"multi_language"})
	lifetimeKey, _ := GenerateTestKey("test@example.com", "lifetime", time.Time{}, "",
		[]string{"multi_language", "priority_support"})

	tests := []struct {
		name       string
		key        string
		wantPro    bool
		wantLife   bool
	}{
		{"Pro license", proKey, true, false},
		{"Lifetime license", lifetimeKey, true, true},
		{"No license", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			m := NewManager(filepath.Join(tmpDir, "license.json"))

			if tt.key != "" {
				if err := m.ValidateKey(tt.key); err != nil {
					t.Fatalf("ValidateKey failed: %v", err)
				}
			}

			if got := m.IsPro(); got != tt.wantPro {
				t.Errorf("IsPro() = %v, want %v", got, tt.wantPro)
			}
			if got := m.IsLifetime(); got != tt.wantLife {
				t.Errorf("IsLifetime() = %v, want %v", got, tt.wantLife)
			}
		})
	}
}

func TestHasFeature(t *testing.T) {
	futureExpiry := time.Now().Add(365 * 24 * time.Hour)

	proKey, _ := GenerateTestKey("test@example.com", "pro", futureExpiry, "",
		[]string{"multi_language", "custom_vocab", "snippets", "cloud_sync", "custom_hotkeys"})
	lifetimeKey, _ := GenerateTestKey("test@example.com", "lifetime", time.Time{}, "",
		[]string{"multi_language", "custom_vocab", "snippets", "cloud_sync", "custom_hotkeys", "priority_support"})

	tests := []struct {
		name      string
		key       string
		feature   string
		want      bool
	}{
		{"Pro has multi_language", proKey, "multi_language", true},
		{"Pro lacks priority_support", proKey, "priority_support", false},
		{"Lifetime has priority_support", lifetimeKey, "priority_support", true},
		{"Lifetime has multi_language", lifetimeKey, "multi_language", true},
		{"Pro lacks nonexistent", proKey, "nonexistent_feature", false},
		{"No license has nothing", "", "multi_language", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			m := NewManager(filepath.Join(tmpDir, "license.json"))

			if tt.key != "" {
				if err := m.ValidateKey(tt.key); err != nil {
					t.Fatalf("ValidateKey failed: %v", err)
				}
			}

			if got := m.HasFeature(tt.feature); got != tt.want {
				t.Errorf("HasFeature(%q) = %v, want %v", tt.feature, got, tt.want)
			}
		})
	}
}

func TestLicensePersistence(t *testing.T) {
	futureExpiry := time.Now().Add(365 * 24 * time.Hour)
	proKey, _ := GenerateTestKey("persist@example.com", "pro", futureExpiry, "",
		[]string{"multi_language", "custom_vocab"})

	tmpDir := t.TempDir()
	licensePath := filepath.Join(tmpDir, "license.json")

	// Save license
	m1 := NewManager(licensePath)
	if err := m1.ValidateKey(proKey); err != nil {
		t.Fatalf("ValidateKey failed: %v", err)
	}

	// Verify file was written
	if _, err := os.Stat(licensePath); err != nil {
		t.Fatalf("license file not created: %v", err)
	}

	// Create new manager from same path — should load from disk
	m2 := NewManager(licensePath)
	lic := m2.GetLicense()
	if lic == nil {
		t.Fatal("expected license to be loaded from disk, got nil")
	}
	if lic.Plan != "pro" {
		t.Errorf("expected plan %q, got %q", "pro", lic.Plan)
	}
	if !lic.Valid {
		t.Error("expected loaded license to be valid")
	}
}

func TestClearLicense(t *testing.T) {
	futureExpiry := time.Now().Add(365 * 24 * time.Hour)
	proKey, _ := GenerateTestKey("clear@example.com", "pro", futureExpiry, "",
		[]string{"multi_language"})

	tmpDir := t.TempDir()
	m := NewManager(filepath.Join(tmpDir, "license.json"))

	if err := m.ValidateKey(proKey); err != nil {
		t.Fatalf("ValidateKey failed: %v", err)
	}
	if !m.IsPro() {
		t.Fatal("expected IsPro to be true after validation")
	}

	if err := m.ClearLicense(); err != nil {
		t.Fatalf("ClearLicense failed: %v", err)
	}
	if m.IsPro() {
		t.Error("expected IsPro to be false after clearing")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "license.json")); !os.IsNotExist(err) {
		t.Error("expected license file to be deleted")
	}
}

func TestFuzzLicenseKey(t *testing.T) {
	t.Skip("Fuzz test — run with: go test -fuzz=FuzzLicenseKey -fuzztime=1m ./internal/license/")
}

func FuzzLicenseKey(f *testing.F) {
	// Seed corpus
	f.Add("yappie-pro-abcdef")
	f.Add("")
	f.Add("a.b")

	f.Fuzz(func(t *testing.T, key string) {
		tmpDir := t.TempDir()
		m := NewManager(filepath.Join(tmpDir, "license.json"))
		// Should never panic
		_ = m.ValidateKey(key)
	})
}

// contains is a simple string contains check (avoiding strings import in some test paths)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}