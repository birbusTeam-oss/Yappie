package tray

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/getlantern/systray"
)

// Status represents the current app state.
type Status int

const (
	StatusIdle Status = iota
	StatusRecording
	StatusTranscribing
	StatusDone
	StatusError
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	pMessageBeep = user32.NewProc("MessageBeep")
)

// SessionStats tracks usage statistics for the current session.
type SessionStats struct {
	mu               sync.Mutex
	TotalWords       int
	TotalDictations  int
	TotalErrors      int
}

func (s *SessionStats) AddDictation(words int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalDictations++
	s.TotalWords += words
}

func (s *SessionStats) AddError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalErrors++
}

func (s *SessionStats) Summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TotalDictations == 0 {
		return "No dictations this session"
	}
	return fmt.Sprintf("%d dictations · %d words", s.TotalDictations, s.TotalWords)
}

// Callbacks for menu actions
type MenuCallbacks struct {
	OnOpenHistory  func()
	OnOpenConfig   func()
	OnOpenLogs     func()
	OnClearHistory func()
}

// Tray manages the system tray icon and menu.
type Tray struct {
	QuitCh    chan struct{}
	hotkey    string
	lastWords int
	stats     *SessionStats
	callbacks MenuCallbacks

	statusItem *systray.MenuItem
	statsItem  *systray.MenuItem
}

// New creates a tray manager.
func New(hotkey string) *Tray {
	return &Tray{
		QuitCh: make(chan struct{}),
		hotkey: hotkey,
		stats:  &SessionStats{},
	}
}

// SetCallbacks configures menu action handlers.
func (t *Tray) SetCallbacks(cb MenuCallbacks) {
	t.callbacks = cb
}

// Stats returns the session stats for external updates.
func (t *Tray) Stats() *SessionStats {
	return t.stats
}

// Run starts the system tray. This BLOCKS — call from main goroutine.
func (t *Tray) Run(onReady func()) {
	systray.Run(func() {
		systray.SetTitle("Yappie")
		systray.SetTooltip("Yappie — Ready (Hold " + t.hotkey + " to dictate)")
		systray.SetIcon(yappieIcon)

		// ── Status ──
		t.statusItem = systray.AddMenuItem("⚡ Ready — Hold "+t.hotkey+" to dictate", "Current status")
		t.statusItem.Disable()

		t.statsItem = systray.AddMenuItem("    No dictations yet", "Session statistics")
		t.statsItem.Disable()

		systray.AddSeparator()

		// ── Actions ──
		mHistory := systray.AddMenuItem("📋 View History", "Open transcription history")
		mConfig := systray.AddMenuItem("⚙️ Open Config", "Edit settings")
		mLogs := systray.AddMenuItem("📄 View Logs", "Open log file")

		systray.AddSeparator()

		// ── Settings ──
		autoStart := systray.AddMenuItemCheckbox("🚀 Start with Windows", "Launch on login", isAutoStartEnabled())

		systray.AddSeparator()

		// ── About / Quit ──
		mAbout := systray.AddMenuItem("ℹ️ Yappie v3.0 — Free & Offline", "Version info")
		mAbout.Disable()

		mQuit := systray.AddMenuItem("✖ Quit Yappie", "Exit application")

		if onReady != nil {
			onReady()
		}

		// Handle menu clicks
		go func() {
			for {
				select {
				case <-autoStart.ClickedCh:
					if autoStart.Checked() {
						autoStart.Uncheck()
						disableAutoStart()
						log.Println("Auto-start disabled")
					} else {
						autoStart.Check()
						enableAutoStart()
						log.Println("Auto-start enabled")
					}

				case <-mHistory.ClickedCh:
					if t.callbacks.OnOpenHistory != nil {
						t.callbacks.OnOpenHistory()
					}

				case <-mConfig.ClickedCh:
					if t.callbacks.OnOpenConfig != nil {
						t.callbacks.OnOpenConfig()
					}

				case <-mLogs.ClickedCh:
					if t.callbacks.OnOpenLogs != nil {
						t.callbacks.OnOpenLogs()
					}

				case <-mQuit.ClickedCh:
					close(t.QuitCh)
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}

// SetStatus updates the tray to reflect current state.
func (t *Tray) SetStatus(s Status) {
	switch s {
	case StatusRecording:
		systray.SetIcon(makeIcon(0xEF, 0x44, 0x44))
		systray.SetTooltip("Yappie — Recording...")
		if t.statusItem != nil {
			t.statusItem.SetTitle("🔴 Recording... (release " + t.hotkey + ")")
		}
	case StatusTranscribing:
		systray.SetIcon(makeIcon(0xF5, 0x9E, 0x0B))
		systray.SetTooltip("Yappie — Transcribing...")
		if t.statusItem != nil {
			t.statusItem.SetTitle("⏳ Transcribing...")
		}
	case StatusDone:
		systray.SetIcon(makeIcon(0x10, 0xB9, 0x81))
		systray.SetTooltip("Yappie — Done!")
		if t.statusItem != nil {
			if t.lastWords > 0 {
				t.statusItem.SetTitle(fmt.Sprintf("✅ Injected %d words", t.lastWords))
			} else {
				t.statusItem.SetTitle("✅ Text injected!")
			}
		}
		// Update session stats display
		if t.statsItem != nil {
			t.statsItem.SetTitle("    " + t.stats.Summary())
		}
		go pMessageBeep.Call(0x00000040)
	case StatusError:
		systray.SetIcon(makeIcon(0xEF, 0x44, 0x44))
		systray.SetTooltip("Yappie — Error")
		if t.statusItem != nil {
			t.statusItem.SetTitle("❌ Error — check logs")
		}
	default: // Idle
		systray.SetIcon(yappieIcon)
		systray.SetTooltip("Yappie — Ready (Hold " + t.hotkey + " to dictate)")
		if t.statusItem != nil {
			t.statusItem.SetTitle("⚡ Ready — Hold " + t.hotkey + " to dictate")
		}
	}
}

// SetLastWordCount stores the word count for display.
func (t *Tray) SetLastWordCount(n int) {
	t.lastWords = n
}

// makeIcon creates a simple colored square ICO.
func makeIcon(r, g, b byte) []byte {
	const size = 16

	ico := []byte{
		0, 0,
		1, 0,
		1, 0,
	}
	ico = append(ico, 16, 16, 0, 0)
	ico = append(ico, 1, 0)
	ico = append(ico, 32, 0)

	ico = append(ico,
		0x28, 0x04, 0x00, 0x00,
	)
	offset := uint32(22)
	ico = append(ico,
		byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24),
	)

	ico = append(ico,
		40, 0, 0, 0,
		byte(size), 0, 0, 0,
		32, 0, 0, 0,
		1, 0,
		32, 0,
		0, 0, 0, 0,
		0x00, 0x04, 0x00, 0x00,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)

	for i := 0; i < size*size; i++ {
		ico = append(ico, b, g, r, 0xFF)
	}

	return ico
}

// Auto-start helpers
func getExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func isAutoStartEnabled() bool {
	cmd := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "Yappie")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run() == nil
}

func enableAutoStart() {
	exe := getExePath()
	if exe == "" {
		return
	}
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "Yappie", "/t", "REG_SZ", "/d", `"`+exe+`"`, "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Run()
}

func disableAutoStart() {
	cmd := exec.Command("reg", "delete",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "Yappie", "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Run()
}
