package ui

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var (
	ColorWhite = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	ColorBlack = color.RGBA{R: 0, G: 0, B: 0, A: 255}
)

func DrawText(img *image.RGBA, x, y int, text string, col color.Color) {
	point := fixed.Point26_6{
		X: fixed.Int26_6(x * 64),
		Y: fixed.Int26_6(y * 64),
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(text)
}

func DrawTextSmall(img *image.RGBA, x, y int, text string, col color.Color) {
	DrawText(img, x, y, text, col)
}

func DrawCenteredText(img *image.RGBA, y int, text string, col color.Color) {
	textWidth := len(text) * 7
	x := (128 - textWidth) / 2
	if x < 0 {
		x = 0
	}
	DrawText(img, x, y, text, col)
}

func DrawInvertedHeader(img *image.RGBA, title string) {
	fillRect(img, 0, 0, 128, 13, ColorWhite)
	textWidth := len(title) * 7
	x := (128 - textWidth) / 2
	if x < 2 {
		x = 2
	}
	DrawText(img, x, 10, title, ColorBlack)
}

func DrawBorder(img *image.RGBA) {
	for x := 0; x < 128; x++ {
		img.Set(x, 0, ColorWhite)
		img.Set(x, 63, ColorWhite)
	}
	for y := 0; y < 64; y++ {
		img.Set(0, y, ColorWhite)
		img.Set(127, y, ColorWhite)
	}
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
	rect := image.Rect(x0, y0, x1, y1)
	draw.Draw(img, rect, &image.Uniform{C: col}, image.Point{}, draw.Src)
}

func WrapText(s string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{s}
	}
	var lines []string
	s = strings.TrimSpace(s)
	for len(s) > maxLen {
		lines = append(lines, s[:maxLen])
		s = s[maxLen:]
	}
	if len(s) > 0 {
		lines = append(lines, s)
	}
	return lines
}
