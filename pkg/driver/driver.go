package driver

import (
	"image"
	"image/color"
)

const DisplayWidth = 128
const DisplayHeight = 64

type DisplayDevice interface {
	Init() error
	Clear() error
	Draw(img image.Image) error
	SetContrast(level byte) error
	PowerOff() error
	PowerOn() error
	Close() error
}

func ImageToPageBuffer(img image.Image, width, height int) []byte {
	pages := height / 8
	buf := make([]byte, pages*width)

	bounds := img.Bounds()
	for p := 0; p < pages; p++ {
		for x := 0; x < width; x++ {
			var b byte
			for bit := 0; bit < 8; bit++ {
				y := p*8 + bit
				imgX := bounds.Min.X + x
				imgY := bounds.Min.Y + y

				if imgX < bounds.Max.X && imgY < bounds.Max.Y {
					c := img.At(imgX, imgY)
					if isPixelOn(c) {
						b |= 1 << bit
					}
				}
			}
			buf[p*width+x] = b
		}
	}
	return buf
}

func isPixelOn(c color.Color) bool {
	if c == nil {
		return false
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return false
	}
	luminance := (299*uint32(r) + 587*uint32(g) + 114*uint32(b)) / 1000
	return luminance > 0x7FFF
}

type MultiDisplay struct {
	devices []DisplayDevice
}

func NewMultiDisplay(devices ...DisplayDevice) *MultiDisplay {
	return &MultiDisplay{
		devices: devices,
	}
}

func (m *MultiDisplay) Init() error {
	for _, d := range m.devices {
		if err := d.Init(); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiDisplay) Clear() error {
	for _, d := range m.devices {
		_ = d.Clear()
	}
	return nil
}

func (m *MultiDisplay) Draw(img image.Image) error {
	for _, d := range m.devices {
		_ = d.Draw(img)
	}
	return nil
}

func (m *MultiDisplay) SetContrast(level byte) error {
	for _, d := range m.devices {
		_ = d.SetContrast(level)
	}
	return nil
}

func (m *MultiDisplay) PowerOff() error {
	for _, d := range m.devices {
		_ = d.PowerOff()
	}
	return nil
}

func (m *MultiDisplay) PowerOn() error {
	for _, d := range m.devices {
		_ = d.PowerOn()
	}
	return nil
}

func (m *MultiDisplay) Close() error {
	for _, d := range m.devices {
		_ = d.Close()
	}
	return nil
}
