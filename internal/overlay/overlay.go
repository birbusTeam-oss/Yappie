package overlay

import (
	"fmt"
	"log"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	pCreateWindowEx             = user32.NewProc("CreateWindowExW")
	pDefWindowProc              = user32.NewProc("DefWindowProcW")
	pRegisterClass              = user32.NewProc("RegisterClassExW")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pGetSysMetrics              = user32.NewProc("GetSystemMetrics")
	pSetTimer                   = user32.NewProc("SetTimer")
	pGetModuleHandle            = kernel32.NewProc("GetModuleHandleW")
	pPeekMessage                = user32.NewProc("PeekMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessage            = user32.NewProc("DispatchMessageW")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
	pSetBkMode          = gdi32.NewProc("SetBkMode")
	pSetTextColor       = gdi32.NewProc("SetTextColor")
	pCreateFont         = gdi32.NewProc("CreateFontW")
	pDrawText           = user32.NewProc("DrawTextW")
	pUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	pGetDC              = user32.NewProc("GetDC")
	pReleaseDC          = user32.NewProc("ReleaseDC")
)

const (
	WS_EX_LAYERED    = 0x00080000
	WS_EX_TOPMOST    = 0x00000008
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_NOACTIVATE = 0x08000000
	WS_POPUP         = 0x80000000
	SW_SHOW          = 5
	SW_HIDE          = 0
	SM_CXSCREEN      = 0
	SM_CYSCREEN      = 1
	WM_TIMER         = 0x0113
	TRANSPARENT      = 1
	DT_CENTER        = 0x01
	DT_VCENTER       = 0x04
	DT_SINGLELINE    = 0x20
	ULW_ALPHA        = 0x02
	AC_SRC_OVER      = 0x00
	AC_SRC_ALPHA     = 0x01

	pillW = 340
	pillH = 44
)

type POINT struct{ X, Y int32 }
type SIZE struct{ CX, CY int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}
type WNDCLASSEX struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}
type BITMAPINFOHEADER struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}
type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type overlayState int

const (
	stateIdle overlayState = iota
	stateRecording
	stateTranscribing
	stateSuccess
	stateError
	stateReady
)

type Overlay struct {
	mu      sync.Mutex
	hwnd    uintptr
	status  string
	r, g, b byte
	state   overlayState
	visible bool
	hideAt  time.Time
	ready   chan struct{}

	// Animation state
	animTick  int
	startTime time.Time

	// Fade animation
	fadeAlpha float64 // 0.0 = invisible, 1.0 = fully visible
	fadeDir   int     // 1 = fading in, -1 = fading out, 0 = stable
}

var globalOverlay *Overlay

func New() *Overlay {
	o := &Overlay{
		r: 0x8B, g: 0x5C, b: 0xF6,
		ready:     make(chan struct{}),
		state:     stateIdle,
		startTime: time.Now(),
		fadeAlpha: 1.0,
	}
	globalOverlay = o
	return o
}

func (o *Overlay) Run() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Overlay recovered from panic: %v", r)
		}
	}()

	className, _ := syscall.UTF16PtrFromString("YappieOverlay")
	hInst, _, _ := pGetModuleHandle.Call(0)

	wc := WNDCLASSEX{
		Size:      uint32(unsafe.Sizeof(WNDCLASSEX{})),
		WndProc:   syscall.NewCallback(overlayWndProc),
		Instance:  hInst,
		ClassName: className,
	}
	pRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))

	screenW, _, _ := pGetSysMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := pGetSysMetrics.Call(SM_CYSCREEN)

	x := (int(screenW) - pillW) / 2
	y := int(screenH) - pillH - 60

	title, _ := syscall.UTF16PtrFromString("")
	hwnd, _, _ := pCreateWindowEx.Call(
		WS_EX_LAYERED|WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		WS_POPUP,
		uintptr(x), uintptr(y), pillW, pillH,
		0, 0, hInst, 0,
	)
	o.hwnd = hwnd

	// Animation timer — 60fps for buttery smooth animations
	pSetTimer.Call(hwnd, 1, 16, 0)

	close(o.ready)

	var msg MSG
	for {
		ret, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
		if ret != 0 {
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
		time.Sleep(4 * time.Millisecond)
	}
}

func overlayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TIMER:
		if globalOverlay != nil {
			globalOverlay.onTick()
		}
	}
	ret, _, _ := pDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return ret
}

func (o *Overlay) onTick() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.visible {
		return
	}

	o.animTick++

	// Handle fade animation
	if o.fadeDir != 0 {
		step := 0.08 // fade speed
		if o.fadeDir > 0 {
			o.fadeAlpha += step
			if o.fadeAlpha >= 1.0 {
				o.fadeAlpha = 1.0
				o.fadeDir = 0
			}
		} else {
			o.fadeAlpha -= step
			if o.fadeAlpha <= 0 {
				o.fadeAlpha = 0
				o.fadeDir = 0
				o.visible = false
				pShowWindow.Call(o.hwnd, SW_HIDE)
				return
			}
		}
	}

	// Auto-hide check
	if !o.hideAt.IsZero() && time.Now().After(o.hideAt) {
		o.fadeDir = -1
		o.hideAt = time.Time{}
	}

	o.renderLocked(pillW, pillH)
}

// ── Public API ──

func (o *Overlay) Show(status string, r, g, b byte, autoHideMs int) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.status = status
	o.r, o.g, o.b = r, g, b
	o.state = stateReady
	o.animTick = 0
	o.startTime = time.Now()
	o.fadeAlpha = 0
	o.fadeDir = 1
	if autoHideMs > 0 {
		o.hideAt = time.Now().Add(time.Duration(autoHideMs) * time.Millisecond)
	} else {
		o.hideAt = time.Time{}
	}
	o.visible = true
	o.renderLocked(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) Hide() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.fadeDir = -1
}

func (o *Overlay) ShowIdle() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.setIdleLocked()
}

func (o *Overlay) setIdleLocked() {
	o.status = ""
	o.r, o.g, o.b = 0x8B, 0x5C, 0xF6
	o.state = stateIdle
	o.visible = true
	o.hideAt = time.Time{}
	o.fadeAlpha = 1.0
	o.fadeDir = 0
	o.renderLocked(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) ShowRecording() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.status = "Listening..."
	o.r, o.g, o.b = 0xEF, 0x44, 0x44
	o.state = stateRecording
	o.animTick = 0
	o.startTime = time.Now()
	o.hideAt = time.Time{}
	o.visible = true
	o.fadeAlpha = 1.0
	o.fadeDir = 0
	o.renderLocked(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) ShowTranscribing() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.status = "Processing..."
	o.r, o.g, o.b = 0xD9, 0x8C, 0x2E
	o.state = stateTranscribing
	o.animTick = 0
	o.startTime = time.Now()
	o.hideAt = time.Time{}
	o.visible = true
	o.fadeAlpha = 1.0
	o.fadeDir = 0
	o.renderLocked(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) ShowSuccess(words int) {
	msg := "Text injected"
	if words == 1 {
		msg = "1 word injected"
	} else if words > 1 {
		msg = fmt.Sprintf("%d words injected", words)
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	o.status = msg
	o.r, o.g, o.b = 0x34, 0xD3, 0x99
	o.state = stateSuccess
	o.animTick = 0
	o.startTime = time.Now()
	o.hideAt = time.Now().Add(2500 * time.Millisecond)
	o.visible = true
	o.fadeAlpha = 1.0
	o.fadeDir = 0
	o.renderLocked(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) ShowError(msg string) {
	if len(msg) > 35 {
		msg = msg[:32] + "..."
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	o.status = msg
	o.r, o.g, o.b = 0xEF, 0x44, 0x44
	o.state = stateError
	o.animTick = 0
	o.startTime = time.Now()
	o.hideAt = time.Now().Add(3 * time.Second)
	o.visible = true
	o.fadeAlpha = 1.0
	o.fadeDir = 0
	o.renderLocked(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) ShowReady(hotkey string) {
	o.Show("Hold "+hotkey+" to dictate", 0x8B, 0x5C, 0xF6, 3000)
}

func (o *Overlay) WaitReady() {
	<-o.ready
}

// ── Rendering ──

func (o *Overlay) renderLocked(w, h int) {
	screenDC, _, _ := pGetDC.Call(0)
	memDC, _, _ := pCreateCompatibleDC.Call(screenDC)

	bmi := BITMAPINFOHEADER{
		Size:     uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		Width:    int32(w),
		Height:   -int32(h),
		Planes:   1,
		BitCount: 32,
	}

	var bits uintptr
	hBmp, _, _ := pCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	pSelectObject.Call(memDC, hBmp)

	pixels := (*[1 << 25]byte)(unsafe.Pointer(bits))

	// Clear
	for i := 0; i < w*h*4; i++ {
		pixels[i] = 0
	}

	// Global fade multiplier
	fade := o.fadeAlpha

	if o.state == stateIdle {
		// Idle: subtle breathing dot — minimal and elegant
		breathe := 0.6 + 0.4*math.Sin(float64(o.animTick)*0.03)
		alpha := byte(float64(65) * breathe * fade)
		drawCircle(pixels, w, h, w/2, h/2, 4, o.r, o.g, o.b, alpha)
		// Outer glow ring
		glowA := byte(float64(20) * breathe * fade)
		drawCircle(pixels, w, h, w/2, h/2, 8, o.r, o.g, o.b, glowA)
	} else {
		// Active states: premium pill with glassmorphism

		// Background — dark frosted glass with subtle gradient
		bgAlpha := byte(float64(225) * fade)
		drawRoundedRect(pixels, w, h, 0, 0, w, h, h/2, 18, 18, 26, bgAlpha)

		// Inner fill gradient — slightly lighter at top for depth
		for y := 0; y < h/2; y++ {
			grad := 1.0 - float64(y)/float64(h/2)
			a := byte(float64(8) * grad * fade)
			for x := 0; x < w; x++ {
				if isInsideRoundedRect(float64(x), float64(y), float64(w), float64(h), float64(h/2)) {
					blendPixel(pixels, w, h, x, y, 255, 255, 255, a)
				}
			}
		}

		// Top highlight — glass reflection
		for y := 1; y < h/3; y++ {
			for x := 0; x < w; x++ {
				if isInsideRoundedRect(float64(x), float64(y), float64(w), float64(h), float64(h/2)) {
					grad := 1.0 - float64(y)/float64(h/3)
					a := byte(float64(18) * grad * grad * fade)
					blendPixel(pixels, w, h, x, y, 255, 255, 255, a)
				}
			}
		}

		// Border — subtle, refined
		borderA := byte(float64(40) * fade)
		drawRoundedRectBorder(pixels, w, h, 0, 0, w, h, h/2, 80, 80, 110, borderA)

		// Inner highlight border (1px inside, very subtle)
		innerA := byte(float64(12) * fade)
		drawRoundedRectBorder(pixels, w, h, 1, 1, w-2, h-2, h/2-1, 255, 255, 255, innerA)

		// Status indicator
		switch o.state {
		case stateRecording:
			o.drawRecordingState(pixels, w, h, fade, memDC)
		case stateTranscribing:
			o.drawTranscribingState(pixels, w, h, fade, memDC)
		case stateSuccess:
			o.drawSuccessState(pixels, w, h, fade, memDC)
		case stateError:
			o.drawErrorState(pixels, w, h, fade, memDC)
		case stateReady:
			o.drawReadyState(pixels, w, h, fade, memDC)
		}
	}

	// Update layered window
	ptSrc := POINT{0, 0}
	sz := SIZE{int32(w), int32(h)}

	screenW, _, _ := pGetSysMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := pGetSysMetrics.Call(SM_CYSCREEN)
	x := (int(screenW) - w) / 2
	y := int(screenH) - h - 60
	ptDst := POINT{int32(x), int32(y)}

	blend := BLENDFUNCTION{
		BlendOp:             AC_SRC_OVER,
		SourceConstantAlpha: 255,
		AlphaFormat:         AC_SRC_ALPHA,
	}

	pUpdateLayeredWindow.Call(
		o.hwnd, screenDC, uintptr(unsafe.Pointer(&ptDst)), uintptr(unsafe.Pointer(&sz)),
		memDC, uintptr(unsafe.Pointer(&ptSrc)), 0,
		uintptr(unsafe.Pointer(&blend)), ULW_ALPHA,
	)

	pDeleteObject.Call(hBmp)
	pDeleteDC.Call(memDC)
	pReleaseDC.Call(0, screenDC)
}

// ── State-specific renderers ──

func (o *Overlay) drawRecordingState(pixels *[1 << 25]byte, w, h int, fade float64, memDC uintptr) {
	// Pulsing red dot with glow
	pulse := 0.5 + 0.5*math.Sin(float64(o.animTick)*0.1)
	dotRadius := 5.0 + 1.0*pulse

	// Outer glow
	glowA := byte(float64(50) * pulse * fade)
	drawCircle(pixels, w, h, 22, h/2, int(dotRadius+5), o.r, o.g, o.b, glowA)
	// Mid glow
	midA := byte(float64(80) * (0.6 + 0.4*pulse) * fade)
	drawCircle(pixels, w, h, 22, h/2, int(dotRadius+2), o.r, o.g, o.b, midA)
	// Core dot
	coreA := byte(float64(255) * (0.8 + 0.2*pulse) * fade)
	drawCircle(pixels, w, h, 22, h/2, int(dotRadius), o.r, o.g, o.b, coreA)

	// Audio waveform bars — 5 bars simulating voice input
	barX := 40
	barSpacing := 5
	barWidth := 3
	numBars := 5
	for i := 0; i < numBars; i++ {
		// Each bar has its own phase for organic movement
		phase := float64(o.animTick)*0.12 + float64(i)*1.3
		amplitude := 0.3 + 0.7*math.Abs(math.Sin(phase))
		maxBarH := 16.0
		barH := int(maxBarH * amplitude)
		if barH < 3 {
			barH = 3
		}

		bx := barX + i*(barWidth+barSpacing)
		by := h/2 - barH/2

		barA := byte(float64(200) * (0.6 + 0.4*amplitude) * fade)
		drawRoundedRect(pixels, w, h, bx, by, barWidth, barH, 1, o.r, o.g, o.b, barA)
	}

	// "Listening..." text
	drawTextGDI(pixels, w, h, o.status, memDC, 78, 0, w-65, h, 0xE0, 0xE0, 0xE8, fade)

	// Timer on the right
	elapsed := time.Since(o.startTime)
	secs := int(elapsed.Seconds())
	timerStr := fmt.Sprintf("%d:%02d", secs/60, secs%60)
	drawTextGDI(pixels, w, h, timerStr, memDC, w-58, 0, w-12, h, 0x88, 0x88, 0x99, fade)
}

func (o *Overlay) drawTranscribingState(pixels *[1 << 25]byte, w, h int, fade float64, memDC uintptr) {
	// Orbiting dots — 3 dots rotating in a circle
	cx, cy := 22, h/2
	for i := 0; i < 3; i++ {
		angle := float64(o.animTick)*0.08 + float64(i)*2.094 // 120 degrees apart
		radius := 5.0
		dx := int(math.Cos(angle) * radius)
		dy := int(math.Sin(angle) * radius)

		// Size varies with position
		dotSize := 2.0 + 0.8*math.Sin(angle+float64(o.animTick)*0.08)
		alpha := byte(float64(220) * (0.5 + 0.5*math.Sin(angle)) * fade)
		drawCircle(pixels, w, h, cx+dx, cy+dy, int(dotSize), o.r, o.g, o.b, alpha)
	}

	// Center dot (amber, steady)
	centerA := byte(float64(140) * fade)
	drawCircle(pixels, w, h, cx, cy, 2, o.r, o.g, o.b, centerA)

	// "Processing..." with animated ellipsis
	dots := int(o.animTick/15) % 4
	text := "Processing" + []string{"", ".", "..", "..."}[dots]
	drawTextGDI(pixels, w, h, text, memDC, 40, 0, w-12, h, 0xE0, 0xE0, 0xE8, fade)
}

func (o *Overlay) drawSuccessState(pixels *[1 << 25]byte, w, h int, fade float64, memDC uintptr) {
	// Green circle with checkmark
	circleA := byte(float64(255) * fade)
	drawCircle(pixels, w, h, 22, h/2, 7, o.r, o.g, o.b, circleA)

	// Soft glow
	glowA := byte(float64(40) * fade)
	drawCircle(pixels, w, h, 22, h/2, 11, o.r, o.g, o.b, glowA)

	// Checkmark
	checkA := byte(float64(255) * fade)
	drawCheck(pixels, w, h, 18, h/2-2, 255, 255, 255, checkA)

	// Success text
	drawTextGDI(pixels, w, h, o.status, memDC, 40, 0, w-12, h, 0xE0, 0xE0, 0xE8, fade)
}

func (o *Overlay) drawErrorState(pixels *[1 << 25]byte, w, h int, fade float64, memDC uintptr) {
	// Red circle with X
	circleA := byte(float64(255) * fade)
	drawCircle(pixels, w, h, 22, h/2, 7, o.r, o.g, o.b, circleA)

	// X mark
	xA := byte(float64(255) * fade)
	drawX(pixels, w, h, 22, h/2, 255, 255, 255, xA)

	// Error text
	drawTextGDI(pixels, w, h, o.status, memDC, 40, 0, w-12, h, 0xE0, 0xE0, 0xE8, fade)
}

func (o *Overlay) drawReadyState(pixels *[1 << 25]byte, w, h int, fade float64, memDC uintptr) {
	// Purple dot
	dotA := byte(float64(200) * fade)
	drawCircle(pixels, w, h, 22, h/2, 5, o.r, o.g, o.b, dotA)

	// Glow
	glowA := byte(float64(50) * fade)
	drawCircle(pixels, w, h, 22, h/2, 9, o.r, o.g, o.b, glowA)

	// Ready text
	drawTextGDI(pixels, w, h, o.status, memDC, 40, 0, w-12, h, 0xCC, 0xCC, 0xDD, fade)
}

// ── Drawing primitives ──

func isInsideRoundedRect(px, py, w, h, r float64) bool {
	return roundedRectDist(px, py, w, h, r) < -0.5
}

func drawCircle(px *[1 << 25]byte, w, h, cx, cy, radius int, r, g, b byte, alpha byte) {
	if alpha == 0 {
		return
	}
	fr := float64(radius)
	for y := clamp(cy-radius-2, 0, h-1); y <= clamp(cy+radius+2, 0, h-1); y++ {
		for x := clamp(cx-radius-2, 0, w-1); x <= clamp(cx+radius+2, 0, w-1); x++ {
			dx := float64(x) - float64(cx)
			dy := float64(y) - float64(cy)
			dist := math.Sqrt(dx*dx+dy*dy) - fr
			if dist < -1.0 {
				blendPixel(px, w, h, x, y, r, g, b, alpha)
			} else if dist < 1.0 {
				a := byte(float64(alpha) * (1.0 - (dist+1.0)/2.0))
				blendPixel(px, w, h, x, y, r, g, b, a)
			}
		}
	}
}

func drawRoundedRect(px *[1 << 25]byte, w, h, rx, ry, rw, rh, radius int, cr, cg, cb byte, alpha byte) {
	if alpha == 0 {
		return
	}
	rad := float64(radius)
	for y := ry; y < ry+rh && y < h; y++ {
		for x := rx; x < rx+rw && x < w; x++ {
			lx := float64(x - rx)
			ly := float64(y - ry)
			fw := float64(rw)
			fh := float64(rh)
			dist := roundedRectDist(lx, ly, fw, fh, rad)
			if dist < -1.0 {
				blendPixel(px, w, h, x, y, cr, cg, cb, alpha)
			} else if dist < 1.0 {
				a := byte(float64(alpha) * (1.0 - (dist+1.0)/2.0))
				blendPixel(px, w, h, x, y, cr, cg, cb, a)
			}
		}
	}
}

func drawRoundedRectBorder(px *[1 << 25]byte, w, h, rx, ry, rw, rh, radius int, cr, cg, cb byte, alpha byte) {
	if alpha == 0 {
		return
	}
	rad := float64(radius)
	for y := ry; y < ry+rh && y < h; y++ {
		for x := rx; x < rx+rw && x < w; x++ {
			lx := float64(x - rx)
			ly := float64(y - ry)
			fw := float64(rw)
			fh := float64(rh)
			dist := roundedRectDist(lx, ly, fw, fh, rad)
			if dist > -1.5 && dist < 0.5 {
				t := 1.0 - math.Abs(dist+0.5)
				if t > 0 {
					a := byte(float64(alpha) * t)
					blendPixel(px, w, h, x, y, cr, cg, cb, a)
				}
			}
		}
	}
}

func roundedRectDist(px, py, w, h, r float64) float64 {
	cx := px - w/2
	cy := py - h/2
	dx := math.Abs(cx) - (w/2 - r)
	dy := math.Abs(cy) - (h/2 - r)
	outside := math.Sqrt(math.Max(dx, 0)*math.Max(dx, 0)+math.Max(dy, 0)*math.Max(dy, 0)) - r
	inside := math.Min(math.Max(dx, dy), 0)
	return outside + inside
}

func drawCheck(px *[1 << 25]byte, stride, h, x, y int, r, g, b, a byte) {
	// Thicker checkmark with antialiasing
	pts := [][2]int{
		{x, y + 2}, {x + 1, y + 3}, {x + 2, y + 4},
		{x + 3, y + 3}, {x + 4, y + 2}, {x + 5, y + 1}, {x + 6, y},
	}
	for _, p := range pts {
		setPixel(px, stride, h, p[0], p[1], r, g, b, a)
		// Thickness: draw pixel above and below
		if p[1]-1 >= 0 {
			setPixel(px, stride, h, p[0], p[1]-1, r, g, b, byte(float64(a)*0.4))
		}
		if p[1]+1 < h {
			setPixel(px, stride, h, p[0], p[1]+1, r, g, b, byte(float64(a)*0.4))
		}
	}
}

func drawX(px *[1 << 25]byte, stride, h, cx, cy int, r, g, b, a byte) {
	for i := -3; i <= 3; i++ {
		setPixel(px, stride, h, cx+i, cy+i, r, g, b, a)
		setPixel(px, stride, h, cx+i, cy-i, r, g, b, a)
		// Slight thickness
		if i > -3 && i < 3 {
			setPixel(px, stride, h, cx+i+1, cy+i, r, g, b, byte(float64(a)*0.3))
			setPixel(px, stride, h, cx+i+1, cy-i, r, g, b, byte(float64(a)*0.3))
		}
	}
}

func setPixel(px *[1 << 25]byte, stride, h, x, y int, r, g, b, a byte) {
	if x < 0 || y < 0 || x >= stride || y >= h {
		return
	}
	off := (y*stride + x) * 4
	fa := float64(a) / 255.0
	px[off+0] = byte(float64(b) * fa)
	px[off+1] = byte(float64(g) * fa)
	px[off+2] = byte(float64(r) * fa)
	px[off+3] = a
}

func blendPixel(px *[1 << 25]byte, stride, h, x, y int, r, g, b, a byte) {
	if x < 0 || y < 0 || x >= stride || y >= h || a == 0 {
		return
	}
	off := (y*stride + x) * 4
	if px[off+3] == 0 {
		setPixel(px, stride, h, x, y, r, g, b, a)
		return
	}
	fa := float64(a) / 255.0
	invA := 1.0 - fa
	px[off+0] = byte(float64(b)*fa + float64(px[off+0])*invA)
	px[off+1] = byte(float64(g)*fa + float64(px[off+1])*invA)
	px[off+2] = byte(float64(r)*fa + float64(px[off+2])*invA)
	newA := float64(a) + float64(px[off+3])*invA
	if newA > 255 {
		newA = 255
	}
	px[off+3] = byte(newA)
}

// drawTextGDI renders text into pixel buffer using GDI with proper alpha compositing.
func drawTextGDI(px *[1 << 25]byte, w, h int, text string, _ uintptr, left, top, right, bottom int, tr, tg, tb byte, fade float64) {
	if text == "" || fade <= 0 {
		return
	}

	screenDC, _, _ := pGetDC.Call(0)
	memDC2, _, _ := pCreateCompatibleDC.Call(screenDC)

	bmi := BITMAPINFOHEADER{
		Size:     uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		Width:    int32(w),
		Height:   -int32(h),
		Planes:   1,
		BitCount: 32,
	}
	var textBits uintptr
	textBmp, _, _ := pCreateDIBSection.Call(memDC2, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&textBits)), 0, 0)
	pSelectObject.Call(memDC2, textBmp)

	textPx := (*[1 << 25]byte)(unsafe.Pointer(textBits))
	for i := 0; i < w*h*4; i++ {
		textPx[i] = 0
	}

	pSetBkMode.Call(memDC2, TRANSPARENT)
	pSetTextColor.Call(memDC2, 0x00FFFFFF)

	fontName, _ := syscall.UTF16PtrFromString("Segoe UI Variable")
	font, _, _ := pCreateFont.Call(
		uintptr(^uint32(15)+1), 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(fontName)),
	)
	oldFont, _, _ := pSelectObject.Call(memDC2, font)

	// Fallback to Segoe UI if Variable isn't available
	textStr, _ := syscall.UTF16PtrFromString(text)
	textRect := RECT{int32(left), int32(top), int32(right), int32(bottom)}
	pDrawText.Call(memDC2, uintptr(unsafe.Pointer(textStr)), ^uintptr(0),
		uintptr(unsafe.Pointer(&textRect)), DT_SINGLELINE|DT_VCENTER)

	// Composite: use brightness as alpha, apply fade
	for y := 0; y < h; y++ {
		for x := left; x < right && x < w; x++ {
			off := (y*w + x) * 4
			r := textPx[off+2]
			g := textPx[off+1]
			b := textPx[off+0]
			mx := r
			if g > mx {
				mx = g
			}
			if b > mx {
				mx = b
			}
			if mx > 10 {
				a := byte(float64(mx) * fade)
				blendPixel(px, w, h, x, y, tr, tg, tb, a)
			}
		}
	}

	pSelectObject.Call(memDC2, oldFont)
	pDeleteObject.Call(font)
	pDeleteObject.Call(textBmp)
	pDeleteDC.Call(memDC2)
	pReleaseDC.Call(0, screenDC)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
