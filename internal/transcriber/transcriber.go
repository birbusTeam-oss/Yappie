package transcriber

import (
	"fmt"
	"log"
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
	// Get our binary's directory (absolute, resolved)
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		resolved, err2 := filepath.EvalSymlinks(exe)
		if err2 == nil {
			exeDir = filepath.Dir(resolved)
		} else {
			exeDir = filepath.Dir(exe)
		}
	}
	log.Printf("[whisper] Binary directory: %s", exeDir)

	if whisperPath == "" && exeDir != "" {
		// Check for whisper.exe next to our binary (ABSOLUTE path only)
		candidate := filepath.Join(exeDir, "whisper.exe")
		log.Printf("[whisper] Checking: %s", candidate)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			whisperPath = candidate
			log.Printf("[whisper] Found: %s", whisperPath)
		} else {
			log.Printf("[whisper] Not found as whisper.exe: %v", err)
			// Check for whisper.bin (shipped renamed to bypass Windows security)
			binCandidate := filepath.Join(exeDir, "whisper.bin")
			if _, err := os.Stat(binCandidate); err == nil {
				log.Printf("[whisper] Found whisper.bin — renaming to whisper.exe")
				if renameErr := os.Rename(binCandidate, candidate); renameErr == nil {
					whisperPath = candidate
					log.Printf("[whisper] Renamed successfully: %s", whisperPath)
				} else {
					log.Printf("[whisper] Rename failed: %v", renameErr)
				}
			}
		}
	}
	if whisperPath == "" && exeDir != "" {
		// Maybe it's called whisper-cli.exe
		candidate := filepath.Join(exeDir, "whisper-cli.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			whisperPath = candidate
			log.Printf("[whisper] Found as whisper-cli.exe: %s", whisperPath)
		}
	}
	if whisperPath == "" {
		log.Printf("[whisper] ERROR: whisper.exe not found anywhere!")
		whisperPath = filepath.Join(exeDir, "whisper.exe") // will fail but with clear error
	}
	log.Printf("[whisper] Final whisper path: %s", whisperPath)

	if modelPath == "" && exeDir != "" {
		// Check next to binary directly
		candidate := filepath.Join(exeDir, "ggml-base.en.bin")
		if _, err := os.Stat(candidate); err == nil {
			modelPath = candidate
			log.Printf("[whisper] Model found: %s", modelPath)
		} else {
			// Check models/ subdirectory
			candidate = filepath.Join(exeDir, "models", "ggml-base.en.bin")
			if _, err := os.Stat(candidate); err == nil {
				modelPath = candidate
				log.Printf("[whisper] Model found in models/: %s", modelPath)
			} else {
				log.Printf("[whisper] Model not found!")
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
