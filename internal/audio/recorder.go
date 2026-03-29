package audio

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// WAV recording using Windows winmm.dll — zero CGO, zero third-party deps.

var (
	winmm            = syscall.NewLazyDLL("winmm.dll")
	waveInOpen       = winmm.NewProc("waveInOpen")
	waveInClose      = winmm.NewProc("waveInClose")
	waveInStart      = winmm.NewProc("waveInStart")
	waveInStop       = winmm.NewProc("waveInStop")
	waveInReset      = winmm.NewProc("waveInReset")
	waveInPrepareHdr = winmm.NewProc("waveInPrepareHeader")
	waveInUnprepareHdr = winmm.NewProc("waveInUnprepareHeader")
	waveInAddBuffer  = winmm.NewProc("waveInAddBuffer")
)

const (
	sampleRate    = 16000
	channels      = 1
	bitsPerSample = 16
	bufferSeconds = 2 // each buffer holds 2s of audio
	maxSeconds    = 120
	waveFormatPCM = 1

	// WAVEHDR flags
	whdrDone     = 0x00000001
	whdrPrepared = 0x00000002

	// callback
	callbackNull = 0x00000000

	waveMapper = 0xFFFFFFFF
)

// WAVEFORMATEX structure
type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// WAVEHDR structure
type waveHdr struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

// Recorder manages mic recording via winmm.
type Recorder struct {
	mu       sync.Mutex
	handle   uintptr
	buffers  []*recordBuffer
	samples  []int16
	recording bool
	stopCh   chan struct{}
}

type recordBuffer struct {
	hdr  *waveHdr
	data []byte
}

// NewRecorder creates a recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Start begins recording from the default mic.
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("already recording")
	}

	wfx := &waveFormatEx{
		FormatTag:      waveFormatPCM,
		Channels:       channels,
		SamplesPerSec:  sampleRate,
		AvgBytesPerSec: sampleRate * channels * bitsPerSample / 8,
		BlockAlign:     channels * bitsPerSample / 8,
		BitsPerSample:  bitsPerSample,
		CbSize:         0,
	}

	var h uintptr
	ret, _, _ := waveInOpen.Call(
		uintptr(unsafe.Pointer(&h)),
		uintptr(waveMapper),
		uintptr(unsafe.Pointer(wfx)),
		0,
		0,
		callbackNull,
	)
	if ret != 0 {
		return fmt.Errorf("waveInOpen failed: %d", ret)
	}

	r.handle = h
	r.samples = r.samples[:0]
	r.stopCh = make(chan struct{})
	r.recording = true

	// Prepare double buffers
	bufSize := sampleRate * channels * (bitsPerSample / 8) * bufferSeconds
	r.buffers = make([]*recordBuffer, 2)
	for i := 0; i < 2; i++ {
		buf := &recordBuffer{
			data: make([]byte, bufSize),
		}
		buf.hdr = &waveHdr{
			Data:         uintptr(unsafe.Pointer(&buf.data[0])),
			BufferLength: uint32(bufSize),
		}
		waveInPrepareHdr.Call(h, uintptr(unsafe.Pointer(buf.hdr)), unsafe.Sizeof(*buf.hdr))
		waveInAddBuffer.Call(h, uintptr(unsafe.Pointer(buf.hdr)), unsafe.Sizeof(*buf.hdr))
		r.buffers[i] = buf
	}

	waveInStart.Call(h)

	go r.collectLoop()
	return nil
}

// collectLoop drains filled buffers and re-queues them.
func (r *Recorder) collectLoop() {
	maxSamples := sampleRate * maxSeconds
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.mu.Lock()
			for _, buf := range r.buffers {
				if buf.hdr.Flags&whdrDone != 0 {
					nBytes := int(buf.hdr.BytesRecorded)
					nSamples := nBytes / 2
					for i := 0; i < nSamples; i++ {
						s := int16(binary.LittleEndian.Uint16(buf.data[i*2:]))
						r.samples = append(r.samples, s)
					}
					buf.hdr.Flags = 0
					buf.hdr.BytesRecorded = 0
					waveInAddBuffer.Call(r.handle, uintptr(unsafe.Pointer(buf.hdr)), unsafe.Sizeof(*buf.hdr))
				}
			}
			if len(r.samples) >= maxSamples {
				r.samples = r.samples[:maxSamples]
				r.mu.Unlock()
				return
			}
			r.mu.Unlock()
		}
	}
}

// Stop ends recording and returns the path to a WAV file.
func (r *Recorder) Stop() (string, error) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return "", fmt.Errorf("not recording")
	}
	r.recording = false
	close(r.stopCh)

	waveInStop.Call(r.handle)
	waveInReset.Call(r.handle)

	// Collect any remaining data
	for _, buf := range r.buffers {
		if buf.hdr.BytesRecorded > 0 {
			nBytes := int(buf.hdr.BytesRecorded)
			nSamples := nBytes / 2
			for i := 0; i < nSamples; i++ {
				s := int16(binary.LittleEndian.Uint16(buf.data[i*2:]))
				r.samples = append(r.samples, s)
			}
		}
		waveInUnprepareHdr.Call(r.handle, uintptr(unsafe.Pointer(buf.hdr)), unsafe.Sizeof(*buf.hdr))
	}

	waveInClose.Call(r.handle)
	samples := make([]int16, len(r.samples))
	copy(samples, r.samples)
	r.mu.Unlock()

	if len(samples) == 0 {
		return "", fmt.Errorf("no audio recorded")
	}

	// Trim silence for faster transcription
	samples = trimSilence(samples, 500)
	if len(samples) < 1600 {
		return "", fmt.Errorf("no speech detected")
	}

	return writeWAV(samples)
}


// trimSilence removes leading/trailing silence from samples.
// Threshold is the minimum amplitude to consider "sound".
func trimSilence(samples []int16, threshold int16) []int16 {
	if len(samples) == 0 {
		return samples
	}

	// Find first non-silent sample
	start := 0
	for start < len(samples) {
		if samples[start] > threshold || samples[start] < -threshold {
			break
		}
		start++
	}

	// Find last non-silent sample
	end := len(samples) - 1
	for end > start {
		if samples[end] > threshold || samples[end] < -threshold {
			break
		}
		end--
	}

	if start >= end {
		return samples // all silence, keep original
	}

	// Add 1600 samples (100ms) padding on each side
	pad := 1600
	start -= pad
	if start < 0 {
		start = 0
	}
	end += pad
	if end >= len(samples) {
		end = len(samples) - 1
	}

	return samples[start : end+1]
}
// writeWAV saves PCM samples to a temp WAV file.
func writeWAV(samples []int16) (string, error) {
	tmpDir := os.TempDir()
	path := filepath.Join(tmpDir, fmt.Sprintf("quill_%d.wav", time.Now().UnixNano()))

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	dataSize := len(samples) * 2
	fileSize := 36 + dataSize

	// RIFF header
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(fileSize))
	f.Write([]byte("WAVE"))

	// fmt chunk
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16)) // chunk size
	binary.Write(f, binary.LittleEndian, uint16(waveFormatPCM))
	binary.Write(f, binary.LittleEndian, uint16(channels))
	binary.Write(f, binary.LittleEndian, uint32(sampleRate))
	binary.Write(f, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8))
	binary.Write(f, binary.LittleEndian, uint16(channels*bitsPerSample/8))
	binary.Write(f, binary.LittleEndian, uint16(bitsPerSample))

	// data chunk
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, uint32(dataSize))
	for _, s := range samples {
		binary.Write(f, binary.LittleEndian, s)
	}

	return path, nil
}
