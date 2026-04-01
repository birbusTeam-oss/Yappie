package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config holds all Yappie settings.
type Config struct {
	Hotkey            string `json:"hotkey"`
	Model             string `json:"model"`
	WhisperPath       string `json:"whisper_path"`
	ModelPath         string `json:"model_path"`
	RemoveFillers     bool   `json:"remove_fillers"`
	LogTranscriptions bool   `json:"log_transcriptions"`

	mu   sync.RWMutex
	path string
}

// DefaultConfig returns config with sane defaults.
func DefaultConfig() *Config {
	return &Config{
		Hotkey:            "ctrl+alt",
		Model:             "base.en",
		WhisperPath:       "whisper.exe",
		ModelPath:         "",
		RemoveFillers:     true,
		LogTranscriptions: true,
	}
}

// configDir returns %APPDATA%/Yappie, creating it if needed.
func configDir() (string, error) {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		appdata = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	dir := filepath.Join(appdata, "Yappie")
	return dir, os.MkdirAll(dir, 0755)
}

// ConfigPath returns the full path to config.json.
func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads config from disk, or returns defaults if file doesn't exist.
func Load() (*Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.path = p
			_ = cfg.Save()
			return cfg, nil
		}
		return DefaultConfig(), err
	}

	cfg := DefaultConfig() // start with defaults so missing fields get filled
	if err := json.Unmarshal(data, cfg); err != nil {
		return DefaultConfig(), err
	}
	cfg.path = p
	return cfg, nil
}

// Save writes config to disk.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p := c.path
	if p == "" {
		var err error
		p, err = ConfigPath()
		if err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// Get returns a thread-safe copy of the hotkey string.
func (c *Config) GetHotkey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Hotkey
}

// SetHotkey updates the hotkey and saves.
func (c *Config) SetHotkey(hk string) error {
	c.mu.Lock()
	c.Hotkey = hk
	c.mu.Unlock()
	return c.Save()
}

// DataDir returns the Yappie data directory (%APPDATA%/Yappie).
func DataDir() (string, error) {
	return configDir()
}
