package ui

import (
	"image"

	"github.com/skip2/go-qrcode"
	"security-hub-display/pkg/observer"
)

type RendererOptions struct {
	QRPrefix string
}

type Renderer struct {
	opts RendererOptions
}

func NewRenderer(opts RendererOptions) *Renderer {
	return &Renderer{
		opts: opts,
	}
}

func (r *Renderer) NumPages(state observer.ServiceState) int {
	if state.Status == observer.StatusSetupRequired && state.Setup != nil {
		return 3
	}
	return 1
}

func (r *Renderer) Render(state observer.ServiceState, pageIndex int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 128, 64))
	fillRect(img, 0, 0, 128, 64, ColorBlack)

	switch state.Status {
	case observer.StatusWaitingForLogs:
		r.renderWaiting(img, state)
	case observer.StatusSetupRequired:
		if state.Setup != nil && state.Setup.Token != "" {
			page := pageIndex % 3
			switch page {
			case 0:
				r.renderSetupQR(img, state.Setup)
			case 1:
				r.renderSetupToken(img, state.Setup)
			case 2:
				r.renderSetupEndpoint(img, state.Setup)
			}
		} else {
			r.renderSetupWaitingToken(img, state)
		}
	case observer.StatusConfigured:
		r.renderConfigured(img, state)
	case observer.StatusError:
		r.renderError(img, state)
	default:
		r.renderWaiting(img, state)
	}

	return img
}

func (r *Renderer) renderWaiting(img *image.RGBA, state observer.ServiceState) {
	DrawInvertedHeader(img, "SECURITY HUB")
	DrawCenteredText(img, 30, "Display Observer", ColorWhite)
	msg := state.Message
	if msg == "" {
		msg = "Waiting for service..."
	}
	DrawCenteredText(img, 46, msg, ColorWhite)
	for x := 10; x < 118; x += 4 {
		img.Set(x, 58, ColorWhite)
	}
}

func (r *Renderer) renderSetupWaitingToken(img *image.RGBA, state observer.ServiceState) {
	DrawInvertedHeader(img, "SETUP REQUIRED")
	DrawCenteredText(img, 30, "SecurityHub API", ColorWhite)
	DrawCenteredText(img, 46, "Waiting for token...", ColorWhite)
	for x := 10; x < 118; x += 4 {
		img.Set(x, 58, ColorWhite)
	}
}

func (r *Renderer) renderSetupQR(img *image.RGBA, info *observer.SetupInfo) {
	qrContent := info.Token
	if r.opts.QRPrefix != "" {
		qrContent = r.opts.QRPrefix + info.Token
	}

	qr, err := qrcode.New(qrContent, qrcode.Low)
	if err == nil {
		bitmap := qr.Bitmap()
		matrixSize := len(bitmap)
		scale := 58 / matrixSize
		if scale < 1 {
			scale = 1
		}
		if scale > 2 {
			scale = 2
		}

		qrPixelSize := matrixSize * scale
		offsetX := 3
		offsetY := (64 - qrPixelSize) / 2

		fillRect(img, offsetX-2, offsetY-2, offsetX+qrPixelSize+2, offsetY+qrPixelSize+2, ColorWhite)

		for y, row := range bitmap {
			for x, val := range row {
				col := ColorWhite
				if val {
					col = ColorBlack
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.Set(offsetX+x*scale+dx, offsetY+y*scale+dy, col)
					}
				}
			}
		}
	}

	fillRect(img, 64, 4, 126, 17, ColorWhite)
	DrawText(img, 68, 14, "SETUP", ColorBlack)

	DrawText(img, 65, 30, "SCAN QR", ColorWhite)
	DrawText(img, 65, 43, "OR TOKEN:", ColorWhite)

	tokenSnippet := info.Token
	if len(tokenSnippet) > 8 {
		tokenSnippet = tokenSnippet[:8] + ".."
	}
	DrawText(img, 65, 57, tokenSnippet, ColorWhite)

	for y := 4; y < 60; y += 2 {
		img.Set(60, y, ColorWhite)
	}
}

func (r *Renderer) renderSetupToken(img *image.RGBA, info *observer.SetupInfo) {
	DrawInvertedHeader(img, "ONE-TIME TOKEN [1/2]")

	lines := WrapText(info.Token, 18)

	startY := 26
	for i, line := range lines {
		if i >= 3 {
			break
		}
		DrawText(img, 2, startY+i*13, line, ColorWhite)
	}
}

func (r *Renderer) renderSetupEndpoint(img *image.RGBA, info *observer.SetupInfo) {
	DrawInvertedHeader(img, "BOOTSTRAP API [2/2]")

	endpoint := info.Endpoint
	if endpoint == "" {
		endpoint = "POST /api/v1/setup/bootstrap"
	}

	lines := WrapText(endpoint, 18)
	startY := 28
	for i, line := range lines {
		if i >= 2 {
			break
		}
		DrawText(img, 2, startY+i*13, line, ColorWhite)
	}

	DrawText(img, 2, 59, "{setup_token,...}", ColorWhite)
}

func (r *Renderer) renderConfigured(img *image.RGBA, state observer.ServiceState) {
	DrawInvertedHeader(img, "SECURITY HUB")

	DrawCenteredText(img, 32, "STATUS: ACTIVE", ColorWhite)
	DrawCenteredText(img, 46, "Configured OK", ColorWhite)

	for x := 4; x < 124; x++ {
		img.Set(x, 56, ColorWhite)
	}
	DrawCenteredText(img, 62, "System Ready", ColorWhite)
}

func (r *Renderer) renderError(img *image.RGBA, state observer.ServiceState) {
	DrawInvertedHeader(img, "SERVICE ALERT")

	msg := state.Message
	if msg == "" {
		msg = "Unexpected error"
	}
	lines := WrapText(msg, 18)
	startY := 28
	for i, line := range lines {
		if i >= 3 {
			break
		}
		DrawText(img, 2, startY+i*13, line, ColorWhite)
	}
}
