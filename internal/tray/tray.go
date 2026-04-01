package tray

import (
	"fmt"
	"log"
	"os"
	"os/exec"
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

// Windows API for balloon notifications
var (
	user32           = syscall.NewLazyDLL("user32.dll")
	shell32          = syscall.NewLazyDLL("shell32.dll")
	pMessageBeep     = user32.NewProc("MessageBeep")
)

// Tray manages the system tray icon and menu.
type Tray struct {
	QuitCh       chan struct{}
	hotkey       string
	statusItem   *systray.MenuItem
	lastWords    int
}

// New creates a tray manager.
func New(hotkey string) *Tray {
	return &Tray{
		QuitCh: make(chan struct{}),
		hotkey: hotkey,
	}
}

// Run starts the system tray. This BLOCKS — call from main goroutine.
func (t *Tray) Run(onReady func()) {
	systray.Run(func() {
		systray.SetTitle("Yappie")
		systray.SetTooltip("Yappie — Ready (Hold " + t.hotkey + " to dictate)")
		systray.SetIcon(yappieIcon) // Yappie icon

		// Status display
		t.statusItem = systray.AddMenuItem("Ready — Hold "+t.hotkey, "")
		t.statusItem.Disable()

		systray.AddSeparator()

		// Auto-start toggle
		autoStart := systray.AddMenuItemCheckbox("Start with Windows", "Launch on login", isAutoStartEnabled())
		
		systray.AddSeparator()

		mQuit := systray.AddMenuItem("Quit Yappie", "Exit")

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
		systray.SetIcon(makeIcon(0xEF, 0x44, 0x44)) // Red = recording
		systray.SetTooltip("Yappie — Recording...")
		if t.statusItem != nil {
			t.statusItem.SetTitle("Recording... (release " + t.hotkey + " to stop)")
		}
	case StatusTranscribing:
		systray.SetIcon(makeIcon(0xF5, 0x9E, 0x0B)) // Amber = processing
		systray.SetTooltip("Yappie — Transcribing...")
		if t.statusItem != nil {
			t.statusItem.SetTitle("Transcribing...")
		}
	case StatusDone:
		systray.SetIcon(makeIcon(0x10, 0xB9, 0x81)) // Green = success
		systray.SetTooltip("Yappie — Done!")
		if t.statusItem != nil {
			if t.lastWords > 0 {
				t.statusItem.SetTitle(fmt.Sprintf("Injected %d words", t.lastWords))
			} else {
				t.statusItem.SetTitle("Text injected!")
			}
		}
		// Play a subtle success sound
		go pMessageBeep.Call(0x00000040) // MB_ICONINFORMATION
	case StatusError:
		systray.SetIcon(makeIcon(0xEF, 0x44, 0x44)) // Red = error
		systray.SetTooltip("Yappie — Error")
		if t.statusItem != nil {
			t.statusItem.SetTitle("Error — check log")
		}
	default: // Idle
		systray.SetIcon(yappieIcon) // Yappie icon
		systray.SetTooltip("Yappie — Ready (Hold " + t.hotkey + " to dictate)")
		if t.statusItem != nil {
			t.statusItem.SetTitle("Ready — Hold " + t.hotkey)
		}
	}
}

// SetLastWordCount stores the word count for display.
func (t *Tray) SetLastWordCount(n int) {
	t.lastWords = n
}

// makeIcon creates a simple colored square ICO.
func makeIcon(r, g, b byte) []byte {
	// 16x16 BMP icon
	const size = 16
	
	// ICO header
	ico := []byte{
		0, 0, // reserved
		1, 0, // type (icon)
		1, 0, // count
	}
	// ICO directory entry
	ico = append(ico, 16, 16, 0, 0) // width, height, palette, reserved
	ico = append(ico, 1, 0)                          // color planes
	ico = append(ico, 32, 0)                         // bits per pixel
	
	// dataSize = 40 (header) + 1024 (pixels) = 1064 // BITMAPINFOHEADER + pixels
	ico = append(ico,
		0x28, 0x04, 0x00, 0x00,
	)
	offset := uint32(22) // offset to BMP data
	ico = append(ico,
		byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24),
	)
	
	// BITMAPINFOHEADER
	ico = append(ico,
		40, 0, 0, 0, // header size
		byte(size), 0, 0, 0, // width
		32, 0, 0, 0, // height (doubled for ICO)
		1, 0, // planes
		32, 0, // bpp
		0, 0, 0, 0, // compression
		0x00, 0x04, 0x00, 0x00,
		0, 0, 0, 0, // x ppm
		0, 0, 0, 0, // y ppm
		0, 0, 0, 0, // colors used
		0, 0, 0, 0, // important colors
	)
	
	// Pixel data (BGRA, bottom-up)
	for i := 0; i < size*size; i++ {
		ico = append(ico, b, g, r, 0xFF) // BGRA
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
