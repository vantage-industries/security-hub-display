package ui

import (
	"testing"
	"time"

	"security-hub-display/pkg/observer"
)

func TestRenderAllStates(t *testing.T) {
	renderer := NewRenderer(RendererOptions{
		QRPrefix: "http://192.168.1.50/setup?token=",
	})

	states := []observer.ServiceState{
		{
			Status:      observer.StatusWaitingForLogs,
			Message:     "Waiting for logs",
			LastUpdated: time.Now(),
		},
		{
			Status: observer.StatusSetupRequired,
			Setup: &observer.SetupInfo{
				Token:        "1blgB3dnXGFfHZJzQ9pbmWJU2c_ESdFSoJnLfgdS1Ak",
				Endpoint:     "POST /api/v1/setup/bootstrap",
				PayloadHint:  "{ setup_token, username, password, full_name }",
				DiscoveredAt: time.Now(),
			},
			LastUpdated: time.Now(),
		},
		{
			Status:      observer.StatusConfigured,
			Message:     "System active",
			LastUpdated: time.Now(),
		},
		{
			Status:      observer.StatusError,
			Message:     "Connection timeout",
			LastUpdated: time.Now(),
		},
	}

	for _, s := range states {
		numPages := renderer.NumPages(s)
		for p := 0; p < numPages; p++ {
			img := renderer.Render(s, p)
			if img == nil {
				t.Fatalf("rendered image is nil for state %v, page %d", s.Status, p)
			}
			bounds := img.Bounds()
			if bounds.Dx() != 128 || bounds.Dy() != 64 {
				t.Errorf("expected 128x64 image, got %dx%d", bounds.Dx(), bounds.Dy())
			}
		}
	}
}
