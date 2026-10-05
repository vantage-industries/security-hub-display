package driver

import (
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
)

type TerminalDisplay struct {
	width  int
	height int
	mu     sync.Mutex
	color  bool
}

func NewTerminalDisplay(color bool) *TerminalDisplay {
	return &TerminalDisplay{
		width:  DisplayWidth,
		height: DisplayHeight,
		color:  color,
	}
}

func (t *TerminalDisplay) Init() error {
	fmt.Print("\033[?25l\033[2J\033[H")
	return nil
}

func (t *TerminalDisplay) Clear() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Print("\033[H")
	var sb strings.Builder
	t.drawBorderTop(&sb)
	for y := 0; y < t.height/2; y++ {
		sb.WriteString("│" + strings.Repeat(" ", t.width) + "│\n")
	}
	t.drawBorderBottom(&sb)
	fmt.Print(sb.String())
	return nil
}

func (t *TerminalDisplay) Draw(img image.Image) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	bounds := img.Bounds()
	var sb strings.Builder

	sb.WriteString("\033[H")
	t.drawBorderTop(&sb)

	colorPrefix := ""
	colorSuffix := ""
	if t.color {
		colorPrefix = "\033[38;2;0;210;255m"
		colorSuffix = "\033[0m"
	}

	for y := 0; y < t.height; y += 2 {
		sb.WriteString("│")
		for x := 0; x < t.width; x++ {
			imgX := bounds.Min.X + x
			imgYTop := bounds.Min.Y + y
			imgYBot := bounds.Min.Y + y + 1

			topOn := false
			botOn := false

			if imgX < bounds.Max.X && imgYTop < bounds.Max.Y {
				topOn = isPixelOn(img.At(imgX, imgYTop))
			}
			if imgX < bounds.Max.X && imgYBot < bounds.Max.Y {
				botOn = isPixelOn(img.At(imgX, imgYBot))
			}

			if topOn && botOn {
				sb.WriteString(colorPrefix + "█" + colorSuffix)
			} else if topOn {
				sb.WriteString(colorPrefix + "▀" + colorSuffix)
			} else if botOn {
				sb.WriteString(colorPrefix + "▄" + colorSuffix)
			} else {
				sb.WriteString(" ")
			}
		}
		sb.WriteString("│\n")
	}

	t.drawBorderBottom(&sb)
	_, err := os.Stdout.WriteString(sb.String())
	return err
}

func (t *TerminalDisplay) drawBorderTop(sb *strings.Builder) {
	sb.WriteString("┌─ [ MOD-08867 1.3\" OLED (128x64) ]" + strings.Repeat("─", t.width-33) + "┐\n")
}

func (t *TerminalDisplay) drawBorderBottom(sb *strings.Builder) {
	sb.WriteString("└" + strings.Repeat("─", t.width) + "┘\n")
}

func (t *TerminalDisplay) SetContrast(level byte) error { return nil }
func (t *TerminalDisplay) PowerOff() error {
	return t.Clear()
}
func (t *TerminalDisplay) PowerOn() error { return nil }

func (t *TerminalDisplay) Close() error {
	fmt.Print("\033[?25h\n")
	return nil
}
