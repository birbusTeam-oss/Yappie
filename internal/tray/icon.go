package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// generateIcon creates a simple 16x16 ICO with a yappie-like design (purple mouth/speech bubble).
func generateIcon() []byte {
	// Create 16x16 RGBA image
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))

	// Fill transparent
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{0, 0, 0, 0})
		}
	}

	// Draw a simple mouth/speech bubble shape
	blue := color.RGBA{66, 133, 244, 255}  // Google blue
	dark := color.RGBA{25, 50, 100, 255}    // dark blue
	gold := color.RGBA{255, 193, 7, 255}    // gold tip

	// Pen body (diagonal line)
	penPixels := [][2]int{
		{3, 12}, {4, 11}, {5, 10}, {6, 9}, {7, 8},
		{8, 7}, {9, 6}, {10, 5}, {11, 4}, {12, 3},
		{4, 12}, {5, 11}, {6, 10}, {7, 9}, {8, 8},
		{9, 7}, {10, 6}, {11, 5}, {12, 4}, {13, 3},
	}
	for _, p := range penPixels {
		img.Set(p[0], p[1], blue)
	}

	// Pen tip
	tipPixels := [][2]int{{2, 13}, {3, 13}, {2, 14}}
	for _, p := range tipPixels {
		img.Set(p[0], p[1], gold)
	}

	// Pen top
	topPixels := [][2]int{{13, 2}, {14, 1}, {13, 1}, {14, 2}}
	for _, p := range topPixels {
		img.Set(p[0], p[1], dark)
	}

	return encodeICO(img)
}

// encodeICO creates a minimal .ico file from a 16x16 RGBA image.
func encodeICO(img *image.RGBA) []byte {
	w := 16
	h := 16

	// BMP pixel data (bottom-up, BGRA)
	var pixelData bytes.Buffer
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			pixelData.WriteByte(byte(b >> 8))
			pixelData.WriteByte(byte(g >> 8))
			pixelData.WriteByte(byte(r >> 8))
			pixelData.WriteByte(byte(a >> 8))
		}
	}

	// AND mask (1 bit per pixel, rows padded to 4 bytes)
	andMaskRowBytes := ((w + 31) / 32) * 4
	andMask := make([]byte, andMaskRowBytes*h)
	// All zeros = all opaque (alpha channel handles transparency)

	bmpInfoSize := 40
	pixelDataLen := pixelData.Len()
	andMaskLen := len(andMask)
	imageSize := bmpInfoSize + pixelDataLen + andMaskLen

	var buf bytes.Buffer

	// ICO header
	binary.Write(&buf, binary.LittleEndian, uint16(0))     // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // count

	// ICO directory entry
	buf.WriteByte(byte(w)) // width (0 = 256)
	buf.WriteByte(byte(h)) // height
	buf.WriteByte(0)       // color palette
	buf.WriteByte(0)       // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // color planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))    // bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(imageSize)) // size
	binary.Write(&buf, binary.LittleEndian, uint32(22))    // offset (6 + 16)

	// BITMAPINFOHEADER
	binary.Write(&buf, binary.LittleEndian, uint32(bmpInfoSize))
	binary.Write(&buf, binary.LittleEndian, int32(w))
	binary.Write(&buf, binary.LittleEndian, int32(h*2)) // height is doubled for ICO
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
	binary.Write(&buf, binary.LittleEndian, uint16(32)) // bpp
	binary.Write(&buf, binary.LittleEndian, uint32(0))  // compression
	binary.Write(&buf, binary.LittleEndian, uint32(pixelDataLen+andMaskLen))
	binary.Write(&buf, binary.LittleEndian, int32(0))   // x ppm
	binary.Write(&buf, binary.LittleEndian, int32(0))   // y ppm
	binary.Write(&buf, binary.LittleEndian, uint32(0))  // colors used
	binary.Write(&buf, binary.LittleEndian, uint32(0))  // important colors

	buf.Write(pixelData.Bytes())
	buf.Write(andMask)

	return buf.Bytes()
}
