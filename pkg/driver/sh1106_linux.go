//go:build linux

package driver

import (
	"fmt"
	"image"
	"os"
	"sync"
	"syscall"
)

const (
	i2cSlave = 0x0703
)

type SH1106Linux struct {
	devPath     string
	i2cAddr     byte
	colOffset   byte
	file        *os.File
	mu          sync.Mutex
	initialized bool
}

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
	if opts.DevPath == "" {
		opts.DevPath = "/dev/i2c-1"
	}
	if opts.Address == 0 {
		opts.Address = 0x3C
	}

	return &SH1106Linux{
		devPath:   opts.DevPath,
		i2cAddr:   opts.Address,
		colOffset: opts.ColumnOffset,
	}, nil
}

func (d *SH1106Linux) Init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	f, err := os.OpenFile(d.devPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open I2C device %s: %w", d.devPath, err)
	}
	d.file = f

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(i2cSlave), uintptr(d.i2cAddr))
	if errno != 0 {
		_ = f.Close()
		d.file = nil
		return fmt.Errorf("failed to set I2C slave address 0x%02X: %w", d.i2cAddr, errno)
	}

	initCommands := []byte{
		0xAE,
		0x02,
		0x10,
		0x40,
		0xB0,
		0x81, 0xCF,
		0xA1,
		0xA6,
		0xA8, 0x3F,
		0xAD, 0x8B,
		0x33,
		0xC8,
		0xD3, 0x00,
		0xD5, 0x80,
		0xD9, 0x1F,
		0xDA, 0x12,
		0xDB, 0x40,
		0xAF,
	}

	for _, cmd := range initCommands {
		if err := d.writeCommand(cmd); err != nil {
			_ = f.Close()
			d.file = nil
			return fmt.Errorf("failed sending init command 0x%02X: %w", cmd, err)
		}
	}

	d.initialized = true
	return d.clearLocked()
}

func (d *SH1106Linux) Clear() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.clearLocked()
}

func (d *SH1106Linux) clearLocked() error {
	emptyPage := make([]byte, DisplayWidth)
	for page := byte(0); page < 8; page++ {
		if err := d.writePage(page, emptyPage); err != nil {
			return err
		}
	}
	return nil
}

func (d *SH1106Linux) Draw(img image.Image) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized || d.file == nil {
		return fmt.Errorf("display device is not initialized")
	}

	pageBuf := ImageToPageBuffer(img, DisplayWidth, DisplayHeight)
	for page := byte(0); page < 8; page++ {
		start := int(page) * DisplayWidth
		end := start + DisplayWidth
		if err := d.writePage(page, pageBuf[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (d *SH1106Linux) SetContrast(level byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.writeCommand(0x81); err != nil {
		return err
	}
	return d.writeCommand(level)
}

func (d *SH1106Linux) PowerOff() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeCommand(0xAE)
}

func (d *SH1106Linux) PowerOn() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeCommand(0xAF)
}

func (d *SH1106Linux) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.file != nil {
		_ = d.writeCommand(0xAE)
		err := d.file.Close()
		d.file = nil
		d.initialized = false
		return err
	}
	return nil
}

func (d *SH1106Linux) writeCommand(cmd byte) error {
	if d.file == nil {
		return fmt.Errorf("I2C device is not open")
	}
	_, err := d.file.Write([]byte{0x00, cmd})
	return err
}

func (d *SH1106Linux) writePage(page byte, data []byte) error {
	if err := d.writeCommand(0xB0 | (page & 0x07)); err != nil {
		return err
	}
	col := d.colOffset
	if err := d.writeCommand(0x00 | (col & 0x0F)); err != nil {
		return err
	}
	if err := d.writeCommand(0x10 | ((col >> 4) & 0x0F)); err != nil {
		return err
	}

	payload := make([]byte, 1+len(data))
	payload[0] = 0x40
	copy(payload[1:], data)

	_, err := d.file.Write(payload)
	return err
}
