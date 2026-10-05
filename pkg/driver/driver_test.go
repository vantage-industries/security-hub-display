package driver

import (
	"image"
	"image/color"
	"testing"
)

func TestImageToPageBuffer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 128, 64))

	img.Set(0, 0, color.White)
	img.Set(0, 7, color.White)
	img.Set(5, 8, color.White)

	buf := ImageToPageBuffer(img, 128, 64)

	if len(buf) != 1024 {
		t.Fatalf("expected 1024 bytes buffer, got %d", len(buf))
	}

	expectedByte0 := byte((1 << 0) | (1 << 7))
	if buf[0] != expectedByte0 {
		t.Errorf("expected buf[0]=0x%02X, got 0x%02X", expectedByte0, buf[0])
	}

	expectedByte133 := byte(1 << 0)
	if buf[133] != expectedByte133 {
		t.Errorf("expected buf[133]=0x%02X, got 0x%02X", expectedByte133, buf[133])
	}
}
