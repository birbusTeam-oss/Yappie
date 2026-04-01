package transcriber

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

// Filler words to remove.
var fillers = map[string]bool{
	"um": true, "uh": true, "er": true, "ah": true,
	"hmm": true, "uhh": true, "umm": true,
}

// Transcriber shells out to a whisper CLI binary.
type Transcriber struct {
	WhisperPath   string // path to whisper executable
	ModelPath     string // path to .bin model file
	RemoveFillers bool
}

// New creates a transcriber. If whisperPath is empty, it looks for whisper.exe
// next to the yappie binary, then in PATH.
func New(whisperPath, modelPath string, removeFillers bool) *Transcriber {
	// If whisperPath from config doesn't actually exist, ignore it
	if whisperPath != "" {
		if _, err := os.Stat(whisperPath); err != nil {
			log.Printf("[whisper] Config path %q doesn't exist, ignoring", whisperPath)
			whisperPath = ""
		}
	}
	// Use %APPDATA%/Yappie as the data directory (survives re-extractions)
	dataDir := filepath.Join(os.Getenv("APPDATA"), "Yappie")
	os.MkdirAll(dataDir, 0755)
	log.Printf("[whisper] Data directory: %s", dataDir)

	whisperExe := filepath.Join(dataDir, "whisper.exe")
	modelFile := filepath.Join(dataDir, "ggml-tiny.en.bin")

	// Check if whisper.exe exists in data dir
	if whisperPath == "" {
		log.Printf("[whisper] Looking for: %s", whisperExe)
		if info, err := os.Stat(whisperExe); err == nil {
			whisperPath = whisperExe
			log.Printf("[whisper] Found: %s (size=%d)", whisperPath, info.Size())
		} else {
			log.Printf("[whisper] Stat error: %v", err)
			// Auto-download whisper.cpp on first run
			log.Printf("[whisper] Not found — downloading whisper.cpp...")
			if err := downloadWhisper(dataDir); err != nil {
				log.Printf("[whisper] Download failed: %v", err)
				whisperPath = "whisper.exe" // will fail but with clear error
			} else {
				whisperPath = whisperExe
				log.Printf("[whisper] Downloaded successfully: %s", whisperPath)
			}
		}
	}

	// Check if model exists
	if modelPath == "" {
		if _, err := os.Stat(modelFile); err == nil {
			modelPath = modelFile
			log.Printf("[whisper] Model found: %s", modelPath)
		} else {
			// Auto-download model
			log.Printf("[whisper] Model not found — downloading...")
			if err := downloadFile("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin", modelFile); err != nil {
				log.Printf("[whisper] Model download failed: %v", err)
			} else {
				modelPath = modelFile
				log.Printf("[whisper] Model downloaded: %s", modelPath)
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
		"-t", "4",           // use 4 threads
		"--no-prints",       // suppress progress output
		"-bs", "1",          // beam size 1 = greedy (fastest)
	}
	if t.ModelPath != "" {
		args = append(args, "-m", t.ModelPath)
	}

	cmd := exec.Command(t.WhisperPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
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


// downloadWhisper downloads and extracts whisper.cpp binaries.
func downloadWhisper(destDir string) error {
	zipURL := "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-bin-x64.zip"
	zipPath := filepath.Join(destDir, "whisper-bin.zip")

	log.Printf("[whisper] Downloading from %s", zipURL)
	if err := downloadFile(zipURL, zipPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(zipPath)

	// Extract whisper-cli.exe and DLLs
	log.Printf("[whisper] Extracting...")
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	needed := map[string]string{
		"Release/whisper-cli.exe": "whisper.exe",
		"Release/ggml-base.dll":  "ggml-base.dll",
		"Release/ggml-cpu.dll":   "ggml-cpu.dll",
		"Release/ggml.dll":       "ggml.dll",
		"Release/whisper.dll":    "whisper.dll",
	}

	for _, f := range r.File {
		destName, ok := needed[f.Name]
		if !ok {
			continue
		}
		destPath := filepath.Join(destDir, destName)
		log.Printf("[whisper] Extracting: %s → %s", f.Name, destPath)

		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", f.Name, err)
		}

		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			return fmt.Errorf("create %s: %w", destPath, err)
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return fmt.Errorf("copy %s: %w", f.Name, err)
		}
	}

	return nil
}

// downloadFile downloads a URL to a local file.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	log.Printf("[whisper] Downloaded %d bytes → %s", written, destPath)
	return nil
}

// Warmup pre-loads the whisper model so the first real transcription is fast.
func (t *Transcriber) Warmup() {
	log.Println("[whisper] Warming up model...")
	// Create a tiny 0.1s silent WAV
	tmpPath := filepath.Join(os.TempDir(), "yappie_warmup.wav")
	f, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("[whisper] Warmup file creation failed: %v", err)
		return
	}
	// Write minimal WAV header + 1600 samples (0.1s at 16kHz)
	samples := 1600
	dataSize := samples * 2
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(36+dataSize))
	f.Write([]byte("WAVE"))
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16))
	binary.Write(f, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(f, binary.LittleEndian, uint16(1)) // mono
	binary.Write(f, binary.LittleEndian, uint32(16000)) // sample rate
	binary.Write(f, binary.LittleEndian, uint32(32000)) // byte rate
	binary.Write(f, binary.LittleEndian, uint16(2)) // block align
	binary.Write(f, binary.LittleEndian, uint16(16)) // bits
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, uint32(dataSize))
	silence := make([]byte, dataSize)
	f.Write(silence)
	f.Close()

	defer os.Remove(tmpPath)

	// Run whisper on it — this loads the model into OS disk cache
	args := []string{
		"-f", tmpPath,
		"--no-timestamps",
		"-l", "en",
		"-t", "4",
		"--no-prints",
		"-bs", "1",
	}
	if t.ModelPath != "" {
		args = append(args, "-m", t.ModelPath)
	}

	cmd := exec.Command(t.WhisperPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	cmd.Run() // ignore errors, just warming up
	os.Remove(tmpPath + ".txt")
	log.Println("[whisper] Warmup complete — model cached")
}
