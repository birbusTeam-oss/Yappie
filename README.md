# Quill ✒️

**Voice dictation for Windows. Offline. Fast. Zero cloud.**

Hold a hotkey, speak, release — your words appear wherever your cursor is. That's it.

![MIT License](https://img.shields.io/badge/license-MIT-purple)
![Go](https://img.shields.io/badge/built%20with-Go-00ADD8)
![Windows](https://img.shields.io/badge/platform-Windows-0078D4)

## Features

- **Fully offline** — runs whisper.cpp locally, nothing leaves your machine
- **Hold-to-talk** — hold Ctrl+Alt (customizable), speak, release to inject text
- **Auto-setup** — first launch downloads whisper.cpp + tiny.en model (~75MB one-time)
- **Floating overlay** — modern dark pill shows recording/transcribing/done status
- **System tray** — lives in your tray, start with Windows option
- **Smart cleanup** — removes filler words (um, uh, er), trims silence
- **Fast** — tiny.en model, greedy decoding, silence trimming, model pre-warming
- **Tiny** — ~6.6MB binary, zero dependencies

## Install

### Option 1: Download from Releases
1. Go to [Releases](https://github.com/birbusTeam-oss/Quill/releases)
2. Download `Quill-Setup.exe` (installer) or `Quill.exe` (portable)
3. Run it

### Option 2: Build from source
```bash
git clone https://github.com/birbusTeam-oss/Quill.git
cd Quill
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui -s -w" -o Quill.exe ./cmd/quill
```

## Usage

1. Launch Quill — you'll see a small purple dot at the bottom of your screen
2. Hold **Ctrl+Alt** and speak
3. Release — text appears at your cursor

The overlay shows status:
- 🟣 Small dot = ready
- 🔴 Expanded pill = recording
- 🟡 Transcribing...
- 🟢 "X words injected" → fades back to dot

## Configuration

Config lives at `%APPDATA%\Quill\config.json`:

```json
{
  "hotkey": "Ctrl+Alt",
  "log_transcriptions": true,
  "remove_fillers": true
}
```

## How It Works

Quill is a single Go binary that:
1. Listens for your hotkey via Windows GetAsyncKeyState API
2. Records from your mic via winmm.dll
3. Trims silence from the audio
4. Sends it to a local whisper.cpp process
5. Cleans up the text (filler removal, capitalization, punctuation)
6. Injects it at your cursor via SendInput

No Python. No Electron. No cloud. Just Go + Win32 API.

## Requirements

- Windows 10/11 (x64)
- A microphone
- ~150MB disk space (whisper binary + model, auto-downloaded)

## License

MIT — do whatever you want with it.

---

Built by [Birbus Team](https://github.com/birbusTeam-oss)
