package overlay

import (
	"fmt"
	"log"
	"math"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	msimg32  = syscall.NewLazyDLL("msimg32.dll")

	pCreateWindowEx         = user32.NewProc("CreateWindowExW")
	pDefWindowProc          = user32.NewProc("DefWindowProcW")
	pRegisterClass          = user32.NewProc("RegisterClassExW")
	pShowWindow             = user32.NewProc("ShowWindow")
	pGetSysMetrics          = user32.NewProc("GetSystemMetrics")
	pInvalidateRect         = user32.NewProc("InvalidateRect")
	pBeginPaint             = user32.NewProc("BeginPaint")
	pEndPaint               = user32.NewProc("EndPaint")
	pSetTimer               = user32.NewProc("SetTimer")
	pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	pGetModuleHandle        = kernel32.NewProc("GetModuleHandleW")
	pPeekMessage            = user32.NewProc("PeekMessageW")
	pTranslateMessage       = user32.NewProc("TranslateMessage")
	pDispatchMessage        = user32.NewProc("DispatchMessageW")
	pMoveWindow             = user32.NewProc("MoveWindow")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pFillRect               = user32.NewProc("FillRect")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pCreateFont             = gdi32.NewProc("CreateFontW")
	pDrawText               = user32.NewProc("DrawTextW")
	pUpdateLayeredWindow    = user32.NewProc("UpdateLayeredWindow")
	pGetDC                  = user32.NewProc("GetDC")
	pReleaseDC              = user32.NewProc("ReleaseDC")
	pAlphaBlend             = msimg32.NewProc("AlphaBlend")
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
	WM_PAINT         = 0x000F
	WM_TIMER         = 0x0113
	TRANSPARENT      = 1
	DT_CENTER        = 0x01
	DT_VCENTER       = 0x04
	DT_SINGLELINE   = 0x20
	ULW_ALPHA        = 0x02
	AC_SRC_OVER      = 0x00
	AC_SRC_ALPHA     = 0x01

	pillW = 260
	pillH = 34
)

type POINT struct{ X, Y int32 }
type SIZE struct{ CX, CY int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type PAINTSTRUCT struct {
	HDC         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}
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

type Overlay struct {
	hwnd    uintptr
	status  string
	r, g, b byte
	visible bool
	hideAt  time.Time
	idle    bool
	ready   chan struct{}
}

var globalOverlay *Overlay

func New() *Overlay {
	o := &Overlay{r: 0x8B, g: 0x5C, b: 0xF6, ready: make(chan struct{})}
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
	y := int(screenH) - pillH - 55

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

	// Timer for auto-hide
	pSetTimer.Call(hwnd, 1, 100, 0)

	// Signal window is ready
	close(o.ready)

	var msg MSG
	for {
		ret, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
		if ret != 0 {
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func overlayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TIMER:
		if globalOverlay != nil && globalOverlay.visible && !globalOverlay.hideAt.IsZero() {
			if time.Now().After(globalOverlay.hideAt) {
				globalOverlay.ShowIdle()
			}
		}
	}
	ret, _, _ := pDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return ret
}

// renderToWindow draws directly using UpdateLayeredWindow for per-pixel alpha.
func (o *Overlay) renderToWindow(w, h int) {
	screenDC, _, _ := pGetDC.Call(0)
	memDC, _, _ := pCreateCompatibleDC.Call(screenDC)

	bmi := BITMAPINFOHEADER{
		Size:     uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		Width:    int32(w),
		Height:   -int32(h), // top-down
		Planes:   1,
		BitCount: 32,
	}

	var bits uintptr
	hBmp, _, _ := pCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	pSelectObject.Call(memDC, hBmp)

	// Get pixel buffer
	pixels := (*[1 << 25]byte)(unsafe.Pointer(bits))

	// Clear to transparent
	for i := 0; i < w*h*4; i++ {
		pixels[i] = 0
	}

	if o.idle {
		// Draw a small soft circle in the center
		drawCircle(pixels, w, h, w/2, h/2, 5, o.r, o.g, o.b, 100)
	} else {
		// Draw rounded pill background
		drawRoundedRect(pixels, w, h, 0, 0, w, h, h/2, 12, 12, 18, 210)
		// Draw border
		drawRoundedRectBorder(pixels, w, h, 0, 0, w, h, h/2, 30, 30, 42, 40)
		// Draw status dot
		drawCircle(pixels, w, h, 18, h/2, 5, o.r, o.g, o.b, 255)
		// Draw text
		drawText(pixels, w, h, o.status, memDC)
	}

	// Update the layered window
	ptSrc := POINT{0, 0}
	sz := SIZE{int32(w), int32(h)}
	
	screenW, _, _ := pGetSysMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := pGetSysMetrics.Call(SM_CYSCREEN)
	x := (int(screenW) - w) / 2
	y := int(screenH) - h - 55
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

// drawCircle draws an anti-aliased filled circle.
func drawCircle(px *[1 << 25]byte, w, h, cx, cy, radius int, r, g, b byte, alpha byte) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x) - float64(cx)
			dy := float64(y) - float64(cy)
			dist := math.Sqrt(dx*dx+dy*dy) - float64(radius)
			if dist < -1.0 {
				// Fully inside
				setPixel(px, w, x, y, r, g, b, alpha)
			} else if dist < 1.0 {
				// Anti-alias edge
				a := byte(float64(alpha) * (1.0 - (dist+1.0)/2.0))
				setPixel(px, w, x, y, r, g, b, a)
			}
		}
	}
}

// drawRoundedRect draws a filled rounded rectangle with anti-aliasing.
func drawRoundedRect(px *[1 << 25]byte, w, h, rx, ry, rw, rh, radius int, cr, cg, cb byte, alpha byte) {
	rad := float64(radius)
	for y := ry; y < ry+rh && y < h; y++ {
		for x := rx; x < rx+rw && x < w; x++ {
			// Distance to rounded rect
			lx := float64(x - rx)
			ly := float64(y - ry)
			fw := float64(rw)
			fh := float64(rh)

			dist := roundedRectDist(lx, ly, fw, fh, rad)
			if dist < -1.0 {
				blendPixel(px, w, x, y, cr, cg, cb, alpha)
			} else if dist < 1.0 {
				a := byte(float64(alpha) * (1.0 - (dist+1.0)/2.0))
				blendPixel(px, w, x, y, cr, cg, cb, a)
			}
		}
	}
}

// drawRoundedRectBorder draws just the border of a rounded rectangle.
func drawRoundedRectBorder(px *[1 << 25]byte, w, h, rx, ry, rw, rh, radius int, cr, cg, cb byte, alpha byte) {
	rad := float64(radius)
	for y := ry; y < ry+rh && y < h; y++ {
		for x := rx; x < rx+rw && x < w; x++ {
			lx := float64(x - rx)
			ly := float64(y - ry)
			fw := float64(rw)
			fh := float64(rh)

			dist := roundedRectDist(lx, ly, fw, fh, rad)
			// Border is between -1.5 and 0.5
			if dist > -1.5 && dist < 0.5 {
				t := 1.0 - math.Abs(dist+0.5)
				if t > 0 {
					a := byte(float64(alpha) * t)
					blendPixel(px, w, x, y, cr, cg, cb, a)
				}
			}
		}
	}
}

// roundedRectDist returns signed distance to a rounded rectangle edge.
func roundedRectDist(px, py, w, h, r float64) float64 {
	// Center the coordinates
	cx := px - w/2
	cy := py - h/2
	// Distance to box with rounded corners
	dx := math.Abs(cx) - (w/2 - r)
	dy := math.Abs(cy) - (h/2 - r)
	outside := math.Sqrt(math.Max(dx, 0)*math.Max(dx, 0)+math.Max(dy, 0)*math.Max(dy, 0)) - r
	inside := math.Min(math.Max(dx, dy), 0)
	return outside + inside
}

func setPixel(px *[1 << 25]byte, stride, x, y int, r, g, b, a byte) {
	off := (y*stride + x) * 4
	// Premultiplied alpha for UpdateLayeredWindow
	fa := float64(a) / 255.0
	px[off+0] = byte(float64(b) * fa)
	px[off+1] = byte(float64(g) * fa)
	px[off+2] = byte(float64(r) * fa)
	px[off+3] = a
}

func blendPixel(px *[1 << 25]byte, stride, x, y int, r, g, b, a byte) {
	off := (y*stride + x) * 4
	if px[off+3] == 0 {
		setPixel(px, stride, x, y, r, g, b, a)
		return
	}
	// Alpha composite (premultiplied)
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

// drawText renders text directly into the pixel buffer as white.
func drawText(px *[1 << 25]byte, w, h int, text string, _ uintptr) {
	// Simple: use GDI to a separate buffer, extract, composite as white
	// Actually, skip GDI entirely. Render using a tiny embedded bitmap font.
	// For now, use GDI but fix the alpha properly.
	
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
	
	// Fill with black
	textPx := (*[1 << 25]byte)(unsafe.Pointer(textBits))
	for i := 0; i < w*h*4; i++ {
		textPx[i] = 0
	}
	
	// Draw white text on black background
	pSetBkMode.Call(memDC2, TRANSPARENT)
	pSetTextColor.Call(memDC2, 0x00FFFFFF) // Pure white
	
	fontName, _ := syscall.UTF16PtrFromString("Segoe UI")
	font, _, _ := pCreateFont.Call(
		uintptr(^uint32(13)+1), 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(fontName)),
	)
	pSelectObject.Call(memDC2, font)
	
	textStr, _ := syscall.UTF16PtrFromString(text)
	textRect := RECT{32, 0, int32(w - 8), int32(h)}
	pDrawText.Call(memDC2, uintptr(unsafe.Pointer(textStr)), ^uintptr(0),
		uintptr(unsafe.Pointer(&textRect)), DT_SINGLELINE|DT_VCENTER)
	
	// Now composite: use the brightness of textPx as alpha for white text
	for y := 0; y < h; y++ {
		for x := 28; x < w-4; x++ {
			off := (y*w + x) * 4
			// GDI rendered white on black — brightness = text coverage
			r := textPx[off+2]
			g := textPx[off+1]
			b := textPx[off+0]
			mx := r
			if g > mx { mx = g }
			if b > mx { mx = b }
			if mx > 15 {
				// Blend white text with alpha = brightness
				a := mx
				blendPixel(px, w, x, y, 0xD4, 0xD4, 0xD8, a)
			}
		}
	}
	
	pDeleteObject.Call(textBmp)
	pDeleteDC.Call(memDC2)
	pReleaseDC.Call(0, screenDC)
}


func (o *Overlay) Show(status string, r, g, b byte, autoHideMs int) {
	o.status = status
	o.r, o.g, o.b = r, g, b
	o.idle = false
	if autoHideMs > 0 {
		o.hideAt = time.Now().Add(time.Duration(autoHideMs) * time.Millisecond)
	} else {
		o.hideAt = time.Time{}
	}
	o.visible = true
	o.renderToWindow(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) Hide() {
	o.visible = false
	pShowWindow.Call(o.hwnd, SW_HIDE)
}

func (o *Overlay) ShowIdle() {
	o.status = ""
	o.r, o.g, o.b = 0x8B, 0x5C, 0xF6
	o.idle = true
	o.visible = true
	o.hideAt = time.Time{}
	o.renderToWindow(pillW, pillH)
	pShowWindow.Call(o.hwnd, SW_SHOW)
}

func (o *Overlay) ShowRecording() {
	o.Show("Recording", 0xE8, 0x56, 0x6D, 0)
}

func (o *Overlay) ShowTranscribing() {
	o.Show("Transcribing...", 0xD9, 0x8C, 0x2E, 0)
}

func (o *Overlay) ShowSuccess(words int) {
	msg := "Text injected"
	if words == 1 {
		msg = "1 word injected"
	} else if words > 1 {
		msg = fmt.Sprintf("%d words injected", words)
	}
	o.Show(msg, 0x34, 0xD3, 0x99, 2000)
}

func (o *Overlay) ShowError(msg string) {
	if len(msg) > 35 {
		msg = msg[:32] + "..."
	}
	o.Show(msg, 0xE8, 0x56, 0x6D, 3000)
}

func (o *Overlay) ShowReady(hotkey string) {
	o.Show("Hold "+hotkey+" to dictate", 0x8B, 0x5C, 0xF6, 3000)
}

// WaitReady blocks until the overlay window is created.
func (o *Overlay) WaitReady() {
	<-o.ready
}
