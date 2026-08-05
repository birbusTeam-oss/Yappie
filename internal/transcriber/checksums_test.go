package transcriber

import (
	"testing"
)

func TestModelChecksumsAllPresent(t *testing.T) {
	// Verify all expected models have checksums
	expectedModels := []string{
		"tiny", "tiny.en", "base", "base.en",
		"small", "small.en", "large-v3-turbo", "large-v3-turbo-q5_0",
	}
	for _, model := range expectedModels {
		sha, ok := modelChecksums[model]
		if !ok {
			t.Errorf("model %q missing from checksums map", model)
			continue
		}
		if len(sha) != 40 {
			t.Errorf("model %q checksum should be 40 hex chars (SHA1), got %d", model, len(sha))
		}
	}
}

func TestModelChecksumsUnknownModel(t *testing.T) {
	sha, ok := modelChecksums["nonexistent"]
	if ok {
		t.Errorf("expected unknown model to not have checksum, got %q", sha)
	}
}

func TestModelDownloadURLsAllPresent(t *testing.T) {
	expectedModels := []string{
		"tiny", "tiny.en", "base", "base.en",
		"small", "small.en", "large-v3-turbo", "large-v3-turbo-q5_0",
	}
	for _, model := range expectedModels {
		url, ok := modelDownloadURLs[model]
		if !ok {
			t.Errorf("model %q missing from download URLs map", model)
			continue
		}
		if url == "" {
			t.Errorf("model %q has empty download URL", model)
		}
		// Verify URL points to huggingface
		if !contains(url, "huggingface.co") {
			t.Errorf("model %q URL should point to huggingface.co, got %q", model, url)
		}
	}
}

func TestWhisperBinaryChecksumExists(t *testing.T) {
	sha, ok := whisperBinaryChecksums["whisper-bin-x64.zip"]
	if !ok {
		t.Error("expected whisper-bin-x64.zip to be in checksums map")
		return
	}
	// It's currently "UNVERIFIED" placeholder — that's documented
	if sha == "" {
		t.Error("expected non-empty checksum (or UNVERIFIED placeholder)")
	}
}

// contains is a helper for string containment check
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}