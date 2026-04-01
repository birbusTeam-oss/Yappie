package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/birbusTeam-oss/Yappie/internal/audio"
	"github.com/birbusTeam-oss/Yappie/internal/config"
	"github.com/birbusTeam-oss/Yappie/internal/history"
	"github.com/birbusTeam-oss/Yappie/internal/hotkey"
	"github.com/birbusTeam-oss/Yappie/internal/injector"
	"github.com/birbusTeam-oss/Yappie/internal/overlay"
	"github.com/birbusTeam-oss/Yappie/internal/snippets"
	"github.com/birbusTeam-oss/Yappie/internal/transcriber"
	"github.com/birbusTeam-oss/Yappie/internal/tray"
)

func main() {
	// Lock main goroutine to OS thread — required for Win32 window message loops
	runtime.LockOSThread()

	// Setup logging
	dataDir, _ := config.DataDir()
	logPath := ""
	if dataDir != "" {
		logPath = filepath.Join(dataDir, "yappie.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
	}
	log.Println("─────────────────────────────────────")
	log.Println("Yappie starting...")
	log.Printf("Version: 3.0.0 | PID: %d", os.Getpid())

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: config load error: %v (using defaults)", err)
	}
	log.Printf("Config loaded — hotkey=%s, fillers=%v, logging=%v",
		cfg.GetHotkey(), cfg.RemoveFillers, cfg.LogTranscriptions)

	// Initialize components
	rec := audio.NewRecorder()
	trans := transcriber.New(cfg.WhisperPath, cfg.ModelPath, cfg.RemoveFillers, func(t *transcriber.Transcriber) {
		t.Language = cfg.Language
		t.Threads = cfg.Threads
	})
	hist := history.New()
	snips := snippets.New()

	// Pre-load whisper model in background
	go trans.Warmup()

	hk := hotkey.New(cfg.GetHotkey())
	tr := tray.New(cfg.GetHotkey())
	ov := overlay.New()
	go ov.Run()

	// Wire up tray menu callbacks
	tr.SetCallbacks(tray.MenuCallbacks{
		OnOpenHistory: func() {
			openHistoryFile(hist)
		},
		OnOpenConfig: func() {
			openInExplorer(cfg)
		},
		OnOpenLogs: func() {
			if logPath != "" {
				openFile(logPath)
			}
		},
	})

	// Main event loop: hotkey → record → transcribe → inject
	var recording bool

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("RECOVERED from panic in event loop: %v", r)
			}
		}()
		for evt := range hk.Events {
			switch evt {
			case hotkey.EventStart:
				if recording {
					continue
				}
				recording = true
				log.Println("Recording started")
				tr.SetStatus(tray.StatusRecording)
				ov.ShowRecording()

				if err := rec.Start(); err != nil {
					log.Printf("Record start error: %v", err)
					tr.SetStatus(tray.StatusError)
					tr.Stats().AddError()
					ov.ShowError("Mic error")
					recording = false
				}

			case hotkey.EventStop:
				if !recording {
					continue
				}
				recording = false
				log.Println("Recording stopped, transcribing...")
				tr.SetStatus(tray.StatusTranscribing)
				ov.ShowTranscribing()

				wavPath, err := rec.Stop()
				if err != nil {
					log.Printf("Record stop error: %v", err)
					if err.Error() == "no speech detected" {
						tr.SetStatus(tray.StatusIdle)
						ov.ShowIdle()
					} else {
						tr.SetStatus(tray.StatusError)
						tr.Stats().AddError()
						ov.ShowError("No audio captured")
					}
					continue
				}

				// Transcribe in background
				go func(wav string) {
					defer os.Remove(wav)
					defer func() {
						if r := recover(); r != nil {
							log.Printf("RECOVERED from panic in transcriber: %v", r)
						}
					}()

					text, err := trans.Transcribe(wav)
					if err != nil {
						log.Printf("Transcription error: %v", err)
						tr.SetStatus(tray.StatusError)
						tr.Stats().AddError()
						ov.ShowError("Transcription failed")
						return
					}

					if text == "" {
						log.Println("Empty transcription")
						tr.SetStatus(tray.StatusIdle)
						ov.ShowIdle()
						return
					}

					// Apply snippet expansion
					text = snips.Expand(text)

					// Count words
					wordCount := countWords(text)

					// Log to history
					if cfg.LogTranscriptions {
						hist.Add(text)
						log.Printf("Transcribed (%d words): %s", wordCount, text)
					}

					// Track session stats
					tr.SetLastWordCount(wordCount)
					tr.Stats().AddDictation(wordCount)

					// Inject into active window
					if err := injector.InjectText(text); err != nil {
						log.Printf("Injection error: %v", err)
						tr.SetStatus(tray.StatusError)
						tr.Stats().AddError()
						ov.ShowError("Paste failed")
						return
					}

					tr.SetStatus(tray.StatusDone)
					ov.ShowSuccess(wordCount)
					log.Println("Text injected successfully")

					// Reset to idle
					time.Sleep(3 * time.Second)
					tr.SetStatus(tray.StatusIdle)
					ov.ShowIdle()
				}(wavPath)
			}
		}
	}()

	// Start hotkey listener
	hk.Start()
	defer hk.Stop()

	// Run tray (blocks)
	tr.Run(func() {
		log.Println("Yappie ready — tray active")
		ov.WaitReady()
		ov.ShowReady(cfg.GetHotkey())
		go func() {
			time.Sleep(3 * time.Second)
			ov.ShowIdle()
		}()
		fmt.Println("Yappie is running. Use the system tray to quit.")
	})

	// Handle tray exit
	log.Println("Tray exited — checking if intentional...")
	select {
	case <-tr.QuitCh:
		log.Println("Yappie shutting down (user quit)")
	default:
		log.Println("Tray crashed — keeping process alive")
		select {}
	}
}

// countWords counts words in text.
func countWords(text string) int {
	count := 0
	inWord := false
	for _, c := range text {
		if c == ' ' || c == '\t' || c == '\n' {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// openHistoryFile exports history to a temp file and opens it.
func openHistoryFile(hist *history.History) {
	entries := hist.GetAll()
	if len(entries) == 0 {
		return
	}

	dir, err := config.DataDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "history_view.txt")

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintln(f, "═══════════════════════════════════════")
	fmt.Fprintln(f, "  Yappie — Transcription History")
	fmt.Fprintln(f, "═══════════════════════════════════════")
	fmt.Fprintln(f)

	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		fmt.Fprintf(f, "  %s  (%d words)\n", e.Timestamp.Format("Jan 2 3:04 PM"), e.WordCount)
		fmt.Fprintf(f, "  %s\n\n", e.Text)
	}

	fmt.Fprintf(f, "───────────────────────────────────────\n")
	fmt.Fprintf(f, "  %d entries total\n", len(entries))

	openFile(path)
}

// openInExplorer opens the config file location.
func openInExplorer(cfg *config.Config) {
	p, err := config.ConfigPath()
	if err != nil {
		return
	}
	openFile(p)
}

// openFile opens a file with the default system handler.
func openFile(path string) {
	cmd := exec.Command("cmd", "/c", "start", "", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Start()
}
