package transcriber

import (
	"testing"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		removeFillers bool
		want          string
	}{
		{"Empty string", "", true, ""},
		{"Whitespace only", "   ", true, ""},
		{"BLANK_AUDIO artifact", "[BLANK_AUDIO]", true, ""},
		{"BLANK_AUDIO with surrounding text", "  [BLANK_AUDIO]  ", true, ""},
		{"Simple text", "hello world", true, "Hello world."},
		{"Already punctuated period", "Hello world.", true, "Hello world."},
		{"Question mark", "is this right?", true, "Is this right?"},
		{"Exclamation", "wow!", true, "Wow!"},
		{"Filler removal on", "um hello uh world", true, "Hello world."},
		{"Filler removal off", "um hello uh world", false, "Um hello uh world."},
		{"Filler with punctuation", "um, hello, uh, world", true, "Hello, world."},
		{"Multi-byte rune", "café", true, "Café."},
		{"Leading/trailing spaces", "  hello  ", true, "Hello."},
		{"Repeated words", "the the test", true, "The the test."},
		{"Newlines in text", "hello\nworld", true, "Hello world."}, // strings.Fields normalizes whitespace
		{"Mixed case", "hELLO wORLD", true, "HELLO wORLD."},
		{"Filler at start", "um hello", true, "Hello."},
		{"Filler at end", "hello um", true, "Hello."},
		{"All fillers", "um uh er ah hmm uhh umm", true, ""},
		{"All fillers no removal", "um uh", false, "Um uh."},
		{"Single word", "test", true, "Test."},
		{"Already capitalized", "Hello world", true, "Hello world."},
		{"Already has exclamation", "Hello!", true, "Hello!"},
		{"Already has question", "Hello?", true, "Hello?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanText(tt.input, tt.removeFillers)
			if got != tt.want {
				t.Errorf("cleanText(%q, %v) = %q, want %q", tt.input, tt.removeFillers, got, tt.want)
			}
		})
	}
}

func TestFillersMap(t *testing.T) {
	expected := []string{"um", "uh", "er", "ah", "hmm", "uhh", "umm"}
	for _, f := range expected {
		if !fillers[f] {
			t.Errorf("expected %q to be in fillers map", f)
		}
	}
	// Verify non-fillers are not in the map
	notFillers := []string{"hello", "world", "test", "the", "a"}
	for _, f := range notFillers {
		if fillers[f] {
			t.Errorf("expected %q to NOT be in fillers map", f)
		}
	}
}

func TestModelChecksums(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		wantSHA   string
		wantOk    bool
	}{
		{"tiny.en", "tiny.en", "c78c86eb1a8faa21b369bcd33207cc90d64ae9df", true},
		{"base.en", "base.en", "137c40403d78fd54d454da0f9bd998f78703390c", true},
		{"tiny", "tiny", "bd577a113a864445d4c299885e0cb97d4ba92b5f", true},
		{"unknown", "nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sha, ok := GetModelChecksum(tt.modelName)
			if ok != tt.wantOk {
				t.Errorf("GetModelChecksum(%q) ok = %v, want %v", tt.modelName, ok, tt.wantOk)
				return
			}
			if ok && sha != tt.wantSHA {
				t.Errorf("GetModelChecksum(%q) = %q, want %q", tt.modelName, sha, tt.wantSHA)
			}
			if ok && len(sha) != 40 {
				t.Errorf("SHA1 should be 40 hex chars, got %d", len(sha))
			}
		})
	}
}

func TestModelDownloadURLs(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		wantOk    bool
	}{
		{"tiny.en", "tiny.en", true},
		{"base", "base", true},
		{"unknown", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, ok := GetModelDownloadURL(tt.modelName)
			if ok != tt.wantOk {
				t.Errorf("GetModelDownloadURL(%q) ok = %v, want %v", tt.modelName, ok, tt.wantOk)
			}
			if ok && url == "" {
				t.Errorf("GetModelDownloadURL(%q) returned empty URL", tt.modelName)
			}
		})
	}
}