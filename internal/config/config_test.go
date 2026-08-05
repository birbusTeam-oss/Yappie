package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Hotkey != "ctrl+alt" {
		t.Errorf("expected default hotkey %q, got %q", "ctrl+alt", cfg.Hotkey)
	}
	if cfg.Model != "tiny.en" {
		t.Errorf("expected default model %q, got %q", "tiny.en", cfg.Model)
	}
	if !cfg.RemoveFillers {
		t.Error("expected RemoveFillers to be true by default")
	}
	if cfg.Threads != 4 {
		t.Errorf("expected default threads 4, got %d", cfg.Threads)
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	yappieDir := filepath.Join(dir, "Yappie")
	os.MkdirAll(yappieDir, 0755)
	path := filepath.Join(yappieDir, "config.json")

	validConfig := `{
		"hotkey": "ctrl+shift",
		"model": "base",
		"language": "es",
		"whisper_path": "/path/to/whisper",
		"model_path": "/path/to/model",
		"remove_fillers": false,
		"threads": 8,
		"log_transcriptions": false,
		"play_sounds": false,
		"auto_capitalize": false,
		"add_punctuation": false
	}`
	os.WriteFile(path, []byte(validConfig), 0644)

	// Set APPDATA so configDir() resolves to our temp dir
	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Hotkey != "ctrl+shift" {
		t.Errorf("expected hotkey %q, got %q", "ctrl+shift", cfg.Hotkey)
	}
	if cfg.Language != "es" {
		t.Errorf("expected language %q, got %q", "es", cfg.Language)
	}
	if cfg.Threads != 8 {
		t.Errorf("expected threads 8, got %d", cfg.Threads)
	}
	if cfg.RemoveFillers {
		t.Error("expected RemoveFillers to be false")
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with missing file should not error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config, got nil")
	}
	if cfg.Hotkey != "ctrl+alt" {
		t.Errorf("expected default hotkey, got %q", cfg.Hotkey)
	}
	// Verify the config file was created
	path := filepath.Join(dir, "Yappie", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected config file to be created: %v", err)
	}
}

func TestLoadMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	yappieDir := filepath.Join(dir, "Yappie")
	os.MkdirAll(yappieDir, 0755)
	path := filepath.Join(yappieDir, "config.json")
	os.WriteFile(path, []byte("{invalid json}"), 0644)

	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg, err := Load()
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
	if cfg == nil {
		t.Fatal("expected default config fallback, got nil")
	}
	if cfg.Hotkey != "ctrl+alt" {
		t.Errorf("expected default hotkey fallback, got %q", cfg.Hotkey)
	}
}

func TestLoadExtraFields(t *testing.T) {
	dir := t.TempDir()
	yappieDir := filepath.Join(dir, "Yappie")
	os.MkdirAll(yappieDir, 0755)
	path := filepath.Join(yappieDir, "config.json")

	extraConfig := `{
		"hotkey": "alt+shift",
		"unknown_field": "should be ignored",
		"another_extra": 42
	}`
	os.WriteFile(path, []byte(extraConfig), 0644)

	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Hotkey != "alt+shift" {
		t.Errorf("expected hotkey %q, got %q", "alt+shift", cfg.Hotkey)
	}
	// Extra fields should be silently ignored (json.Unmarshal ignores unknown fields)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg := DefaultConfig()
	cfg.Hotkey = "ctrl+shift+space"
	cfg.Threads = 12
	cfg.Language = "fr"
	cfg.path = filepath.Join(dir, "Yappie", "config.json")
	os.MkdirAll(filepath.Dir(cfg.path), 0755)

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(cfg.path)
	if err != nil {
		t.Fatalf("config file not found: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	if raw["hotkey"] != "ctrl+shift+space" {
		t.Errorf("expected saved hotkey %q, got %v", "ctrl+shift+space", raw["hotkey"])
	}

	// Load again and verify values preserved
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg2.Hotkey != "ctrl+shift+space" {
		t.Errorf("expected loaded hotkey %q, got %q", "ctrl+shift+space", cfg2.Hotkey)
	}
	if cfg2.Threads != 12 {
		t.Errorf("expected loaded threads 12, got %d", cfg2.Threads)
	}
	if cfg2.Language != "fr" {
		t.Errorf("expected loaded language %q, got %q", "fr", cfg2.Language)
	}
}

func TestConfigValidation(t *testing.T) {
	dir := t.TempDir()
	yappieDir := filepath.Join(dir, "Yappie")
	os.MkdirAll(yappieDir, 0755)
	path := filepath.Join(yappieDir, "config.json")

	// Threads < 1 should be corrected to 4
	badConfig := `{
		"hotkey": "ctrl+alt",
		"threads": 0,
		"language": ""
	}`
	os.WriteFile(path, []byte(badConfig), 0644)

	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Threads != 4 {
		t.Errorf("expected threads corrected to 4, got %d", cfg.Threads)
	}
	if cfg.Language != "en" {
		t.Errorf("expected language corrected to 'en', got %q", cfg.Language)
	}
}

func TestGetSetHotkey(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("APPDATA", dir)
	defer os.Unsetenv("APPDATA")

	cfg := DefaultConfig()
	cfg.path = filepath.Join(dir, "Yappie", "config.json")
	os.MkdirAll(filepath.Dir(cfg.path), 0755)

	if err := cfg.SetHotkey("alt+shift"); err != nil {
		t.Fatalf("SetHotkey failed: %v", err)
	}
	if got := cfg.GetHotkey(); got != "alt+shift" {
		t.Errorf("GetHotkey() = %q, want %q", got, "alt+shift")
	}
}