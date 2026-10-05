//go:build !linux

package driver

import (
	"fmt"
	"image"
)

type SH1106Linux struct{}

type SH1106Options struct {
	DevPath      string
	Address      byte
	ColumnOffset byte
}

func DefaultSH1106Options() SH1106Options {
	return SH1106Options{
		DevPath:      "/dev/i2c-1",
		Address:      0x3C,
		ColumnOffset: 2,
	}
}

func NewSH1106(opts SH1106Options) (*SH1106Linux, error) {
	return nil, fmt.Errorf("physical I2C hardware is only supported on Linux (use --mode=terminal or --mode=web on macOS)")
}

func (d *SH1106Linux) Init() error                 { return nil }
func (d *SH1106Linux) Clear() error                { return nil }
func (d *SH1106Linux) Draw(img image.Image) error  { return nil }
func (d *SH1106Linux) SetContrast(level byte) error { return nil }
func (d *SH1106Linux) PowerOff() error             { return nil }
func (d *SH1106Linux) PowerOn() error              { return nil }
func (d *SH1106Linux) Close() error                { return nil }
