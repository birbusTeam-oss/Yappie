package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/birbusTeam-oss/quill/internal/audio"
	"github.com/birbusTeam-oss/quill/internal/config"
	"github.com/birbusTeam-oss/quill/internal/history"
	"github.com/birbusTeam-oss/quill/internal/hotkey"
	"github.com/birbusTeam-oss/quill/internal/injector"
	"github.com/birbusTeam-oss/quill/internal/snippets"
	"github.com/birbusTeam-oss/quill/internal/transcriber"
	"github.com/birbusTeam-oss/quill/internal/overlay"
	"github.com/birbusTeam-oss/quill/internal/tray"
)

func main() {
	// Setup logging
	dataDir, _ := config.DataDir()
	if dataDir != "" {
		logPath := filepath.Join(dataDir, "quill.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
	}
	log.Println("Quill starting...")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: config load error: %v (using defaults)", err)
	}

	// Initialize components
	rec := audio.NewRecorder()
	trans := transcriber.New(cfg.WhisperPath, cfg.ModelPath, cfg.RemoveFillers)
	hist := history.New()
	snips := snippets.New()
	// Pre-load whisper model in background for faster first transcription
	go trans.Warmup()

	hk := hotkey.New(cfg.GetHotkey())
	tr := tray.New(cfg.GetHotkey())
	ov := overlay.New()
	go ov.Run()

	// Main logic: wire hotkey events to record/transcribe/inject pipeline
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
					tr.SetStatus(tray.StatusError)
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

					// Log to history
					if cfg.LogTranscriptions {
						hist.Add(text)
						log.Printf("Transcribed: %s", text)
					}

					// Track word count
				wordCount := 0
				for _, c := range text {
					if c == ' ' {
						wordCount++
					}
				}
				wordCount++ // last word
				tr.SetLastWordCount(wordCount)

				// Inject into active window
					if err := injector.InjectText(text); err != nil {
						log.Printf("Injection error: %v", err)
						tr.SetStatus(tray.StatusError)
						return
					}

					tr.SetStatus(tray.StatusDone)
					ov.ShowSuccess(wordCount)
					log.Println("Text injected successfully")

					// Reset to idle after a moment
					time.Sleep(2 * time.Second)
					tr.SetStatus(tray.StatusIdle)
					ov.ShowIdle()
				}(wavPath)
			}
		}
	}()

	// Start hotkey listener
	hk.Start()
	defer hk.Stop()

	// Run tray (blocks until quit)
	tr.Run(func() {
		log.Println("Quill ready — tray active")
		// Wait for overlay window to be created
		ov.WaitReady()
		ov.ShowReady(cfg.GetHotkey())
		go func() {
			time.Sleep(3 * time.Second)
			ov.ShowIdle()
		}()
		fmt.Println("Quill is running. Use the system tray to quit.")
	})

	// If tray exits unexpectedly, keep running and try to restart it
	log.Println("Tray exited — checking if intentional...")
	select {
	case <-tr.QuitCh:
		log.Println("Quill shutting down (user quit)")
	default:
		log.Println("Tray crashed — keeping process alive")
		// Block forever — only system shutdown or task kill will stop us
		select {}
	}
}
