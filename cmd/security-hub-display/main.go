package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"security-hub-display/pkg/driver"
	"security-hub-display/pkg/observer"
	"security-hub-display/pkg/status"
	"security-hub-display/pkg/ui"
)

func main() {
	var (
		unitName         = flag.String("unit", getEnv("SECURITY_HUB_UNIT", "security-hub-api"), "systemd unit name to observe via journalctl")
		statusURL        = flag.String("status-url", getEnv("STATUS_URL", "http://127.0.0.1:8080/setup/status"), "URL to /setup/status endpoint (empty to disable HTTP check)")
		pollIntervalSec  = flag.Int("poll-interval", 2, "HTTP status poll interval in seconds")
		displayMode      = flag.String("mode", getEnv("DISPLAY_MODE", defaultMode()), "display mode: 'i2c' (MOD-08867), 'terminal', 'web', or 'all'")
		i2cBus           = flag.String("i2c-bus", getEnv("I2C_BUS", "/dev/i2c-1"), "Linux I2C device bus path")
		i2cAddrStr       = flag.String("i2c-addr", getEnv("I2C_ADDR", "0x3C"), "I2C slave address (e.g. 0x3C or 0x3D)")
		colOffset        = flag.Int("col-offset", 2, "SH1106 column RAM offset (2 for MOD-08867 SH1106, 0 for SSD1306)")
		useStdin         = flag.Bool("stdin", false, "read logs directly from stdin instead of running journalctl")
		mockMode         = flag.Bool("mock", false, "simulate security-hub setup logs and status API for testing")
		qrPrefix         = flag.String("qr-prefix", getEnv("QR_PREFIX", ""), "URL prefix for QR code (e.g. 'http://security-hub.local/setup?token=')")
		carouselSec      = flag.Int("carousel", 5, "carousel page rotation interval in seconds")
		webPort          = flag.Int("web-port", 8085, "HTTP preview port for 'web' or 'all' mode")
		termColor        = flag.Bool("color", true, "use ANSI colors in terminal mode")
		dimTimeoutSec    = flag.Int("dim-timeout", 0, "screen turn-off timeout in seconds after inactivity (0 = always on)")
	)
	flag.Parse()

	log.Printf("Starting SecurityHub Display Observer (v1.1.0)...")
	log.Printf("Target Service Unit: %s", *unitName)
	log.Printf("Status Endpoint: %s", *statusURL)
	log.Printf("Display Mode: %s", *displayMode)

	if *mockMode && *statusURL != "" {
		mockServerURL := startMockStatusServer()
		*statusURL = mockServerURL
		log.Printf("[mock] Mock status server running at %s", *statusURL)
	}

	parsedAddr, err := parseHexOrDec(*i2cAddrStr)
	if err != nil {
		log.Fatalf("Invalid --i2c-addr %q: %v", *i2cAddrStr, err)
	}

	disp, err := setupDisplay(*displayMode, *i2cBus, byte(parsedAddr), byte(*colOffset), *termColor, *webPort)
	if err != nil {
		log.Fatalf("Failed to initialize display: %v", err)
	}
	defer disp.Close()

	if err := disp.Init(); err != nil {
		log.Fatalf("Display init error: %v", err)
	}
	defer disp.Clear()

	renderer := ui.NewRenderer(ui.RendererOptions{
		QRPrefix: *qrPrefix,
	})

	obsCfg := observer.Config{
		UnitName: *unitName,
		UseStdin: *useStdin,
		Mock:     *mockMode,
		Reader:   os.Stdin,
	}
	obs := observer.New(obsCfg)
	obs.Start()
	defer obs.Stop()

	var statusClient *status.Client
	if *statusURL != "" {
		statusClient = status.NewClient(*statusURL, 2*time.Second)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	carouselTicker := time.NewTicker(time.Duration(*carouselSec) * time.Second)
	defer carouselTicker.Stop()

	pollInterval := time.Duration(*pollIntervalSec) * time.Second
	if pollInterval < 500*time.Millisecond {
		pollInterval = 500 * time.Millisecond
	}
	statusTicker := time.NewTicker(pollInterval)
	defer statusTicker.Stop()

	var (
		mu              sync.Mutex
		currentState    = obs.CurrentState()
		cachedSetup     *observer.SetupInfo
		apiSaysRequired *bool
		currentPage     = 0
		lastActivity    = time.Now()
		screenDimmed    = false
	)

	redraw := func() {
		mu.Lock()
		defer mu.Unlock()

		if screenDimmed {
			return
		}
		frame := renderer.Render(currentState, currentPage)
		_ = disp.Draw(frame)
	}

	redraw()

	checkStatus := func() {
		if statusClient == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		res := statusClient.Check(ctx)
		cancel()

		mu.Lock()
		defer mu.Unlock()

		if !res.Available {
			if apiSaysRequired == nil {
				currentState = observer.ServiceState{
					Status:      observer.StatusWaitingForLogs,
					Message:     "Connecting to API...",
					LastUpdated: time.Now(),
				}
			}
			return
		}

		apiSaysRequired = &res.SetupRequired

		if res.SetupRequired {
			if currentState.Status != observer.StatusSetupRequired {
				log.Printf("[status-api] setup_required = true -> Showing setup token/QR")
				currentState = observer.ServiceState{
					Status:      observer.StatusSetupRequired,
					Setup:       cachedSetup,
					Message:     "Setup required: token active",
					LastUpdated: time.Now(),
				}
				currentPage = 0
				lastActivity = time.Now()
				if screenDimmed {
					screenDimmed = false
					_ = disp.PowerOn()
				}
				frame := renderer.Render(currentState, currentPage)
				_ = disp.Draw(frame)
			}
		} else {
			if currentState.Status != observer.StatusConfigured {
				log.Printf("[status-api] setup_required = false -> Setup completed!")
				currentState = observer.ServiceState{
					Status:      observer.StatusConfigured,
					Message:     "SecurityHub is configured and active",
					LastUpdated: time.Now(),
				}
				currentPage = 0
				lastActivity = time.Now()
				if screenDimmed {
					screenDimmed = false
					_ = disp.PowerOn()
				}
				frame := renderer.Render(currentState, currentPage)
				_ = disp.Draw(frame)
			}
		}
	}

	go checkStatus()

	for {
		select {
		case <-sigChan:
			log.Println("Shutting down display observer...")
			_ = disp.Clear()
			return

		case obsState := <-obs.Updates():
			mu.Lock()
			if obsState.Setup != nil && obsState.Setup.Token != "" {
				cachedSetup = obsState.Setup
			}

			if statusClient != nil && apiSaysRequired != nil {
				if *apiSaysRequired {
					currentState = observer.ServiceState{
						Status:      observer.StatusSetupRequired,
						Setup:       cachedSetup,
						Message:     "Setup required: token detected",
						LastUpdated: time.Now(),
					}
					currentPage = 0
					lastActivity = time.Now()
				} else {
					currentState = observer.ServiceState{
						Status:      observer.StatusConfigured,
						Message:     "SecurityHub is configured",
						LastUpdated: time.Now(),
					}
				}
			} else {
				currentState = obsState
				currentPage = 0
				lastActivity = time.Now()
			}

			if screenDimmed {
				screenDimmed = false
				_ = disp.PowerOn()
			}
			mu.Unlock()

			redraw()

		case <-statusTicker.C:
			checkStatus()

		case <-carouselTicker.C:
			mu.Lock()
			if *dimTimeoutSec > 0 && !screenDimmed {
				if time.Since(lastActivity) > time.Duration(*dimTimeoutSec)*time.Second {
					screenDimmed = true
					_ = disp.PowerOff()
					mu.Unlock()
					continue
				}
			}

			if screenDimmed {
				mu.Unlock()
				continue
			}

			numPages := renderer.NumPages(currentState)
			if numPages > 1 {
				currentPage = (currentPage + 1) % numPages
				frame := renderer.Render(currentState, currentPage)
				_ = disp.Draw(frame)
			}
			mu.Unlock()
		}
	}
}

func startMockStatusServer() string {
	startTime := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		isSetup := time.Since(startTime) < 12*time.Second
		fmt.Fprintf(w, `{"setup_required": %t}`, isSetup)
	}))
	return server.URL + "/setup/status"
}

func setupDisplay(mode, i2cBus string, i2cAddr byte, colOffset byte, color bool, webPort int) (driver.DisplayDevice, error) {
	switch strings.ToLower(mode) {
	case "i2c", "hardware", "sh1106":
		return driver.NewSH1106(driver.SH1106Options{
			DevPath:      i2cBus,
			Address:      i2cAddr,
			ColumnOffset: colOffset,
		})

	case "terminal", "term", "cli":
		return driver.NewTerminalDisplay(color), nil

	case "web", "http":
		return driver.NewWebPreviewDisplay(webPort), nil

	case "all":
		var devices []driver.DisplayDevice
		devices = append(devices, driver.NewTerminalDisplay(color))
		devices = append(devices, driver.NewWebPreviewDisplay(webPort))
		if runtime.GOOS == "linux" {
			if hw, err := driver.NewSH1106(driver.SH1106Options{
				DevPath:      i2cBus,
				Address:      i2cAddr,
				ColumnOffset: colOffset,
			}); err == nil {
				devices = append(devices, hw)
			}
		}
		return driver.NewMultiDisplay(devices...), nil

	default:
		return nil, fmt.Errorf("unknown display mode %q (expected 'i2c', 'terminal', 'web', or 'all')", mode)
	}
}

func defaultMode() string {
	if runtime.GOOS == "linux" {
		return "i2c"
	}
	return "terminal"
}

func parseHexOrDec(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseUint(s[2:], 16, 8)
	}
	return strconv.ParseUint(s, 10, 8)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
