package driver

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"sync"
	"time"
)

type WebPreviewDisplay struct {
	port        int
	server      *http.Server
	mu          sync.RWMutex
	lastImage   image.Image
	lastUpdate  time.Time
	subscribers map[chan []byte]struct{}
	subMu       sync.Mutex
}

func NewWebPreviewDisplay(port int) *WebPreviewDisplay {
	if port <= 0 {
		port = 8085
	}
	return &WebPreviewDisplay{
		port:        port,
		subscribers: make(map[chan []byte]struct{}),
		lastImage:   image.NewRGBA(image.Rect(0, 0, DisplayWidth, DisplayHeight)),
	}
}

func (w *WebPreviewDisplay) Init() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleIndex)
	mux.HandleFunc("/screen.png", w.handleScreenPNG)
	mux.HandleFunc("/events", w.handleSSE)

	w.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", w.port),
		Handler: mux,
	}

	go func() {
		log.Printf("[webpreview] OLED Web Simulator live at http://localhost:%d", w.port)
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[webpreview] Server error: %v", err)
		}
	}()

	return nil
}

func (w *WebPreviewDisplay) Clear() error {
	img := image.NewRGBA(image.Rect(0, 0, DisplayWidth, DisplayHeight))
	for y := 0; y < DisplayHeight; y++ {
		for x := 0; x < DisplayWidth; x++ {
			img.Set(x, y, color.Black)
		}
	}
	return w.Draw(img)
}

func (w *WebPreviewDisplay) Draw(img image.Image) error {
	w.mu.Lock()
	w.lastImage = img
	w.lastUpdate = time.Now()
	w.mu.Unlock()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err == nil {
		pngBytes := buf.Bytes()
		w.subMu.Lock()
		for ch := range w.subscribers {
			select {
			case ch <- pngBytes:
			default:
			}
		}
		w.subMu.Unlock()
	}

	return nil
}

func (w *WebPreviewDisplay) SetContrast(level byte) error { return nil }
func (w *WebPreviewDisplay) PowerOff() error             { return w.Clear() }
func (w *WebPreviewDisplay) PowerOn() error              { return nil }

func (w *WebPreviewDisplay) Close() error {
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

func (w *WebPreviewDisplay) handleIndex(rw http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MOD-08867 OLED Display Simulator</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            background-color: #0b0f19;
            color: #e2e8f0;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            background: #151d2f;
            border: 1px solid #1e293b;
            border-radius: 12px;
            padding: 30px;
            box-shadow: 0 10px 25px rgba(0,0,0,0.5);
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 20px;
        }
        h1 { font-size: 1.25rem; font-weight: 600; color: #94a3b8; letter-spacing: 0.05em; }
        .bezel {
            background: #020617;
            padding: 16px;
            border-radius: 8px;
            border: 2px solid #334155;
            box-shadow: inset 0 2px 10px rgba(0,0,0,0.8);
        }
        .screen-canvas {
            display: block;
            width: 512px;
            height: 256px;
            image-rendering: pixelated;
            background: #000;
            border-radius: 2px;
            box-shadow: 0 0 20px rgba(0, 195, 255, 0.25);
        }
        .meta {
            font-size: 0.85rem;
            color: #64748b;
            display: flex;
            gap: 20px;
        }
        .badge {
            background: #0f172a;
            border: 1px solid #334155;
            padding: 4px 10px;
            border-radius: 9999px;
            font-family: monospace;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>MOD-08867 1.3" OLED SIMULATOR (128x64)</h1>
        <div class="bezel">
            <canvas id="screen" class="screen-canvas" width="128" height="64"></canvas>
        </div>
        <div class="meta">
            <span class="badge">Driver: SH1106</span>
            <span class="badge">Resolution: 128x64</span>
            <span class="badge" id="status-badge">Connecting...</span>
        </div>
    </div>
    <script>
        const canvas = document.getElementById('screen');
        const ctx = canvas.getContext('2d');
        const statusBadge = document.getElementById('status-badge');

        function updateScreen() {
            const img = new Image();
            img.onload = () => {
                ctx.fillStyle = '#000000';
                ctx.fillRect(0, 0, 128, 64);
                ctx.drawImage(img, 0, 0);
                statusBadge.textContent = 'Live • Connected';
                statusBadge.style.color = '#38bdf8';
            };
            img.onerror = () => {
                statusBadge.textContent = 'Reconnecting...';
                statusBadge.style.color = '#f87171';
            };
            img.src = '/screen.png?t=' + Date.now();
        }

        setInterval(updateScreen, 250);
        updateScreen();
    </script>
</body>
</html>`
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = rw.Write([]byte(html))
}

func (w *WebPreviewDisplay) handleScreenPNG(rw http.ResponseWriter, r *http.Request) {
	w.mu.RLock()
	img := w.lastImage
	w.mu.RUnlock()

	rw.Header().Set("Content-Type", "image/png")
	rw.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	if img != nil {
		_ = png.Encode(rw, img)
	}
}

func (w *WebPreviewDisplay) handleSSE(rw http.ResponseWriter, r *http.Request) {
	flusher, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 2)
	w.subMu.Lock()
	w.subscribers[ch] = struct{}{}
	w.subMu.Unlock()

	defer func() {
		w.subMu.Lock()
		delete(w.subscribers, ch)
		w.subMu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprintf(rw, "data: updated\n\n")
			flusher.Flush()
		}
	}
}
