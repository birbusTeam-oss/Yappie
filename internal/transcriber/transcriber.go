package transcriber

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// Filler words to remove.
var fillers = map[string]bool{
	"um": true, "uh": true, "er": true, "ah": true,
	"like": true, "you know": true, "i mean": true,
	"basically": true, "actually": true, "literally": true,
	"so": true, "well": true, "right": true,
}

// Transcriber shells out to a whisper CLI binary.
type Transcriber struct {
	WhisperPath   string // path to whisper executable
	ModelPath     string // path to .bin model file
	RemoveFillers bool
}

// New creates a transcriber. If whisperPath is empty, it looks for whisper.exe
// next to the quill binary, then in PATH.
func New(whisperPath, modelPath string, removeFillers bool) *Transcriber {
	if whisperPath == "" {
		// Look next to our binary first (use absolute resolved path)
		exe, err := os.Executable()
		if err == nil {
			exe, _ = filepath.EvalSymlinks(exe)
			candidate := filepath.Join(filepath.Dir(exe), "whisper.exe")
			absCandidate, _ := filepath.Abs(candidate)
			if _, err := os.Stat(absCandidate); err == nil {
				whisperPath = absCandidate
			}
		}
		if whisperPath == "" {
			// Try current working directory
			if cwd, err := os.Getwd(); err == nil {
				candidate := filepath.Join(cwd, "whisper.exe")
				if _, err := os.Stat(candidate); err == nil {
					whisperPath = candidate
				}
			}
		}
		if whisperPath == "" {
			// Last resort: look in PATH
			if found, err := exec.LookPath("whisper.exe"); err == nil {
				whisperPath = found
			} else {
				whisperPath = "whisper.exe"
			}
		}
	}
	if modelPath == "" {
		exe, err2 := os.Executable()
		if err2 == nil {
			exe, _ = filepath.EvalSymlinks(exe)
			// Look for models/ dir next to binary
			modelsDir := filepath.Join(filepath.Dir(exe), "models")
			candidate := filepath.Join(modelsDir, "ggml-base.en.bin")
			if _, err := os.Stat(candidate); err == nil {
				modelPath = candidate
			}
		}
	}
	return &Transcriber{
		WhisperPath:   whisperPath,
		ModelPath:     modelPath,
		RemoveFillers: removeFillers,
	}
}

// Transcribe runs whisper on the given WAV file and returns cleaned text.
func (t *Transcriber) Transcribe(wavPath string) (string, error) {
	args := []string{
		"-f", wavPath,
		"--no-timestamps",
		"--output-txt",
		"-l", "en",
	}
	if t.ModelPath != "" {
		args = append(args, "-m", t.ModelPath)
	}

	cmd := exec.Command(t.WhisperPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper failed: %w\noutput: %s", err, string(out))
	}

	// whisper might output to a .txt file next to the wav
	txtPath := wavPath + ".txt"
	var text string
	if data, err := os.ReadFile(txtPath); err == nil {
		text = string(data)
		os.Remove(txtPath) // cleanup
	} else {
		// Or it might print to stdout
		text = string(out)
	}

	text = cleanText(text, t.RemoveFillers)
	return text, nil
}

// cleanText trims, capitalizes, removes fillers, ensures punctuation.
func cleanText(text string, removeFillers bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Remove [BLANK_AUDIO] or similar whisper artifacts
	text = strings.ReplaceAll(text, "[BLANK_AUDIO]", "")
	text = strings.TrimSpace(text)

	if removeFillers {
		words := strings.Fields(text)
		var cleaned []string
		for _, w := range words {
			lower := strings.ToLower(strings.Trim(w, ".,!?;:"))
			if !fillers[lower] {
				cleaned = append(cleaned, w)
			}
		}
		text = strings.Join(cleaned, " ")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Capitalize first letter
	runes := []rune(text)
	runes[0] = unicode.ToUpper(runes[0])
	text = string(runes)

	// Ensure ends with punctuation
	last := text[len(text)-1]
	if last != '.' && last != '!' && last != '?' {
		text += "."
	}

	return text
}
