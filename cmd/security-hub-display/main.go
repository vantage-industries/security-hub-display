package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
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

const (
	contrastDim    = 0x05 // Low power / dim glow to protect OLED phosphor
	contrastBright = 0xCF // Standard bright contrast for setup and tests
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "test" || os.Args[1] == "--test" || os.Args[1] == "-test") {
		runTestCommand(os.Args[2:])
		return
	}

	var (
		unitName        = flag.String("unit", getEnv("SECURITY_HUB_UNIT", "security-hub-api"), "systemd unit name to observe via journalctl")
		statusURL       = flag.String("status-url", getEnv("STATUS_URL", "http://127.0.0.1:8080/setup/status"), "URL to /setup/status endpoint (empty to disable HTTP check)")
		pollIntervalSec = flag.Int("poll-interval", 2, "HTTP status poll interval in seconds")
		displayMode     = flag.String("mode", getEnv("DISPLAY_MODE", defaultMode()), "display mode: 'i2c' (MOD-08867), 'terminal', 'web', or 'all'")
		i2cBus          = flag.String("i2c-bus", getEnv("I2C_BUS", "/dev/i2c-1"), "Linux I2C device bus path")
		i2cAddrStr      = flag.String("i2c-addr", getEnv("I2C_ADDR", "0x3C"), "I2C slave address (e.g. 0x3C or 0x3D)")
		colOffset       = flag.Int("col-offset", 2, "SH1106 column RAM offset (2 for MOD-08867 SH1106, 0 for SSD1306)")
		useStdin        = flag.Bool("stdin", false, "read logs directly from stdin instead of running journalctl")
		mockMode        = flag.Bool("mock", false, "simulate security-hub setup logs and status API for testing")
		qrPrefix        = flag.String("qr-prefix", getEnv("QR_PREFIX", ""), "URL prefix for QR code (e.g. 'http://security-hub.local/setup?token=')")
		carouselSec     = flag.Int("carousel", 5, "carousel page rotation interval in seconds")
		webPort         = flag.Int("web-port", 8085, "HTTP preview port for 'web' or 'all' mode")
		termColor       = flag.Bool("color", true, "use ANSI colors in terminal mode")
		isTestFlag      = flag.Bool("test", false, "trigger test mode")
	)
	flag.Parse()

	if *isTestFlag {
		runTestCommand(nil)
		return
	}

	log.Printf("Starting SecurityHub Display Observer (v1.3.0)...")
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
	defer func() {
		_ = disp.Clear()
		_ = disp.PowerOff()
	}()

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

	usr1Chan := make(chan os.Signal, 1)
	signal.Notify(usr1Chan, syscall.SIGUSR1)

	testTrigger := make(chan time.Duration, 5)
	powerTrigger := make(chan bool, 5)

	sockListener := startControlSocket(defaultSocketPath(), testTrigger, powerTrigger)
	if sockListener != nil {
		defer func() {
			_ = sockListener.Close()
			_ = os.Remove(defaultSocketPath())
		}()
	}

	carouselTicker := time.NewTicker(time.Duration(*carouselSec) * time.Second)
	defer carouselTicker.Stop()

	pollInterval := time.Duration(*pollIntervalSec) * time.Second
	if pollInterval < 500*time.Millisecond {
		pollInterval = 500 * time.Millisecond
	}
	statusTicker := time.NewTicker(pollInterval)
	defer statusTicker.Stop()

	var (
		mu               sync.Mutex
		currentState     = obs.CurrentState()
		cachedSetup      *observer.SetupInfo
		apiSaysRequired  *bool
		displayTurnOn    = true // Enabled by default, can be toggled via API or socket
		currentPage      = 0
		screenPoweredOn  = false
		testActive       = false
		testActiveUntil  time.Time
		localIP          = detectOutboundIP()
		formattedBusInfo = fmt.Sprintf("%s @0x%02X", filepath.Base(*i2cBus), parsedAddr)
		connectedDevices int64
	)

	applyBrightness := func() {
		if testActive || currentState.Status == observer.StatusSetupRequired {
			_ = disp.SetContrast(contrastBright)
		} else if currentState.Status == observer.StatusConfigured {
			_ = disp.SetContrast(contrastDim)
		}
	}

	turnOnDisplay := func() {
		if !screenPoweredOn {
			_ = disp.PowerOn()
			screenPoweredOn = true
		}
		applyBrightness()
	}

	turnOffDisplay := func() {
		if screenPoweredOn {
			_ = disp.Clear()
			_ = disp.PowerOff()
			screenPoweredOn = false
		}
	}

	redraw := func() {
		mu.Lock()
		defer mu.Unlock()

		if testActive {
			turnOnDisplay()
			frame := renderer.Render(currentState, currentPage)
			_ = disp.Draw(frame)
			return
		}

		if !displayTurnOn {
			// User turned off display via endpoint/socket
			turnOffDisplay()
			return
		}

		if apiSaysRequired != nil && !*apiSaysRequired {
			// Appliance configured -> DIM status display
			turnOnDisplay()
			frame := renderer.Render(currentState, 0)
			_ = disp.Draw(frame)
			return
		}

		// Setup required or waiting for logs -> full brightness setup carousel
		turnOnDisplay()
		frame := renderer.Render(currentState, currentPage)
		_ = disp.Draw(frame)
	}

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
			if apiSaysRequired == nil && !testActive {
				currentState = observer.ServiceState{
					Status:      observer.StatusWaitingForLogs,
					Message:     "Connecting to API...",
					LastUpdated: time.Now(),
				}
				if displayTurnOn {
					turnOnDisplay()
					frame := renderer.Render(currentState, currentPage)
					_ = disp.Draw(frame)
				}
			}
			return
		}

		apiSaysRequired = &res.SetupRequired
		displayTurnOn = res.TurnOn
		connectedDevices = res.ConnectedDevices
		if res.IPAddress != "" {
			localIP = res.IPAddress
		}

		if testActive {
			// Keep test pattern until timer expires
			return
		}

		if !displayTurnOn {
			turnOffDisplay()
			return
		}

		if res.SetupRequired {
			if currentState.Status != observer.StatusSetupRequired {
				log.Printf("[status-api] setup_required = true -> Showing setup token/QR code")
				currentState = observer.ServiceState{
					Status:      observer.StatusSetupRequired,
					Setup:       cachedSetup,
					Message:     "Setup required: token active",
					LastUpdated: time.Now(),
				}
				currentPage = 0
				turnOnDisplay()
				frame := renderer.Render(currentState, currentPage)
				_ = disp.Draw(frame)
			}
		} else {
			// Configured state -> DIM SecurityHUB, IP, and connected devices
			currentState = observer.ServiceState{
				Status: observer.StatusConfigured,
				Configured: &observer.ConfiguredInfo{
					IPAddress:        localIP,
					ConnectedDevices: connectedDevices,
					TurnOn:           displayTurnOn,
				},
				Message:     "SecurityHub active",
				LastUpdated: time.Now(),
			}
			currentPage = 0
			turnOnDisplay()
			frame := renderer.Render(currentState, 0)
			_ = disp.Draw(frame)
		}
	}

	go checkStatus()

	for {
		select {
		case <-sigChan:
			log.Println("Shutting down display observer...")
			turnOffDisplay()
			return

		case <-usr1Chan:
			log.Println("[signal] SIGUSR1 received -> Triggering 30s display test")
			testTrigger <- 30 * time.Second

		case turnOn := <-powerTrigger:
			mu.Lock()
			displayTurnOn = turnOn
			log.Printf("[control-socket] Display power state set to: turn_on=%v", turnOn)
			if !turnOn && !testActive {
				turnOffDisplay()
			} else {
				turnOnDisplay()
				frame := renderer.Render(currentState, currentPage)
				_ = disp.Draw(frame)
			}
			mu.Unlock()

		case dur := <-testTrigger:
			mu.Lock()
			log.Printf("[test-trigger] Activating display test for %v", dur)
			testActive = true
			testActiveUntil = time.Now().Add(dur)
			currentState = observer.ServiceState{
				Status: observer.StatusTest,
				Test: &observer.TestInfo{
					ActiveUntil: testActiveUntil,
					DeviceBus:   formattedBusInfo,
					DeviceAddr:  fmt.Sprintf("0x%02X", parsedAddr),
					IPAddress:   localIP,
					Message:     "TEST ACTIVE",
				},
				LastUpdated: time.Now(),
			}
			currentPage = 0
			turnOnDisplay()
			frame := renderer.Render(currentState, currentPage)
			_ = disp.Draw(frame)
			mu.Unlock()

		case obsState := <-obs.Updates():
			mu.Lock()
			if obsState.Setup != nil && obsState.Setup.Token != "" {
				cachedSetup = obsState.Setup
			}

			if !testActive && displayTurnOn {
				if statusClient != nil && apiSaysRequired != nil {
					if *apiSaysRequired {
						currentState = observer.ServiceState{
							Status:      observer.StatusSetupRequired,
							Setup:       cachedSetup,
							Message:     "Setup required: token detected",
							LastUpdated: time.Now(),
						}
						currentPage = 0
						turnOnDisplay()
					} else {
						currentState = observer.ServiceState{
							Status: observer.StatusConfigured,
							Configured: &observer.ConfiguredInfo{
								IPAddress:        localIP,
								ConnectedDevices: connectedDevices,
								TurnOn:           displayTurnOn,
							},
							Message:     "SecurityHub active",
							LastUpdated: time.Now(),
						}
						turnOnDisplay()
					}
				} else {
					currentState = obsState
					currentPage = 0
					turnOnDisplay()
				}
			}
			mu.Unlock()

			redraw()

		case <-statusTicker.C:
			checkStatus()

		case <-carouselTicker.C:
			mu.Lock()
			if testActive {
				if time.Now().After(testActiveUntil) {
					log.Printf("[test] Test duration finished. Reverting display state.")
					testActive = false
					if !displayTurnOn {
						turnOffDisplay()
					} else if apiSaysRequired != nil && *apiSaysRequired {
						currentState = observer.ServiceState{
							Status:      observer.StatusSetupRequired,
							Setup:       cachedSetup,
							Message:     "Setup required: token active",
							LastUpdated: time.Now(),
						}
						currentPage = 0
						turnOnDisplay()
						frame := renderer.Render(currentState, currentPage)
						_ = disp.Draw(frame)
					} else {
						currentState = observer.ServiceState{
							Status: observer.StatusConfigured,
							Configured: &observer.ConfiguredInfo{
								IPAddress:        localIP,
								ConnectedDevices: connectedDevices,
								TurnOn:           displayTurnOn,
							},
							Message:     "SecurityHub active",
							LastUpdated: time.Now(),
						}
						turnOnDisplay()
						frame := renderer.Render(currentState, 0)
						_ = disp.Draw(frame)
					}
					mu.Unlock()
					continue
				}

				numPages := renderer.NumPages(currentState)
				if numPages > 1 {
					currentPage = (currentPage + 1) % numPages
					turnOnDisplay()
					frame := renderer.Render(currentState, currentPage)
					_ = disp.Draw(frame)
				}
				mu.Unlock()
				continue
			}

			if !displayTurnOn {
				turnOffDisplay()
				mu.Unlock()
				continue
			}

			if apiSaysRequired != nil && !*apiSaysRequired {
				// Configured: keep showing the DIM status screen (no rotation needed)
				turnOnDisplay()
				frame := renderer.Render(currentState, 0)
				_ = disp.Draw(frame)
				mu.Unlock()
				continue
			}

			// Unconfigured: rotate carousel
			numPages := renderer.NumPages(currentState)
			if numPages > 1 {
				currentPage = (currentPage + 1) % numPages
				turnOnDisplay()
				frame := renderer.Render(currentState, currentPage)
				_ = disp.Draw(frame)
			}
			mu.Unlock()
		}
	}
}

func runTestCommand(args []string) {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	duration := fs.Duration("duration", 30*time.Second, "Test display duration")
	displayMode := fs.String("mode", defaultMode(), "Display mode ('i2c', 'terminal', 'web')")
	i2cBus := fs.String("i2c-bus", getEnv("I2C_BUS", "/dev/i2c-1"), "Linux I2C bus path")
	i2cAddrStr := fs.String("i2c-addr", getEnv("I2C_ADDR", "0x3C"), "I2C slave address")
	colOffset := fs.Int("col-offset", 2, "Column RAM offset")
	termColor := fs.Bool("color", true, "Use ANSI colors in terminal mode")
	sockPath := fs.String("sock", defaultSocketPath(), "Path to daemon control socket")
	fs.Parse(args)

	// 1. Try sending test command to running background service
	if tryNotifyDaemon(*sockPath, fmt.Sprintf("TEST %s", duration.String())) {
		fmt.Printf("[security-hub-display] Sent test command to running display service (active for %v).\n", *duration)
		return
	}

	// 2. Otherwise run standalone test directly on hardware/terminal
	fmt.Printf("[security-hub-display] No running service on %s; running standalone display test (%v)...\n", *sockPath, *duration)
	runStandaloneTest(*displayMode, *i2cBus, *i2cAddrStr, *colOffset, *termColor, *duration)
}

func tryNotifyDaemon(sockPath string, cmd string) bool {
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = fmt.Fprintf(conn, "%s\n", cmd)
	if err != nil {
		return false
	}

	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}
	resp := strings.TrimSpace(string(buf[:n]))
	return strings.HasPrefix(resp, "OK")
}

func runStandaloneTest(mode, bus, addrStr string, colOffset int, color bool, duration time.Duration) {
	parsedAddr, err := parseHexOrDec(addrStr)
	if err != nil {
		log.Fatalf("Invalid --i2c-addr %q: %v", addrStr, err)
	}

	disp, err := setupDisplay(mode, bus, byte(parsedAddr), byte(colOffset), color, 8085)
	if err != nil {
		log.Fatalf("Failed to initialize display: %v", err)
	}
	defer disp.Close()

	if err := disp.Init(); err != nil {
		log.Fatalf("Display init error: %v", err)
	}
	_ = disp.PowerOn()
	_ = disp.SetContrast(contrastBright)

	renderer := ui.NewRenderer(ui.RendererOptions{})
	ip := detectOutboundIP()
	state := observer.ServiceState{
		Status: observer.StatusTest,
		Test: &observer.TestInfo{
			ActiveUntil: time.Now().Add(duration),
			DeviceBus:   fmt.Sprintf("%s @0x%02X", filepath.Base(bus), parsedAddr),
			DeviceAddr:  fmt.Sprintf("0x%02X", parsedAddr),
			IPAddress:   ip,
			Message:     "STANDALONE TEST",
		},
		LastUpdated: time.Now(),
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	page := 0
	_ = disp.Draw(renderer.Render(state, page))

	deadline := time.Now().Add(duration)
	for {
		select {
		case <-sigChan:
			fmt.Println("\nStopping display test...")
			_ = disp.Clear()
			_ = disp.PowerOff()
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				fmt.Println("Display test complete.")
				_ = disp.Clear()
				_ = disp.PowerOff()
				return
			}
			page = (page + 1) % renderer.NumPages(state)
			_ = disp.Draw(renderer.Render(state, page))
		}
	}
}

func startControlSocket(sockPath string, testTrigger chan<- time.Duration, powerTrigger chan<- bool) net.Listener {
	_ = os.Remove(sockPath)
	_ = os.MkdirAll(filepath.Dir(sockPath), 0755)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Printf("[control-socket] Warning: cannot listen on %s: %v", sockPath, err)
		return nil
	}
	_ = os.Chmod(sockPath, 0666)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go handleControlConn(conn, testTrigger, powerTrigger)
		}
	}()
	return l
}

func handleControlConn(conn net.Conn, testTrigger chan<- time.Duration, powerTrigger chan<- bool) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)
		if len(parts) > 0 {
			cmd := strings.ToUpper(parts[0])
			switch cmd {
			case "TEST":
				dur := 30 * time.Second
				if len(parts) > 1 {
					if d, err := time.ParseDuration(parts[1]); err == nil && d > 0 {
						dur = d
					}
				}
				testTrigger <- dur
				fmt.Fprintf(conn, "OK: test active for %v\n", dur)
				return
			case "ON", "TURN_ON", "1", "TRUE":
				powerTrigger <- true
				fmt.Fprintf(conn, "OK: display turned on\n")
				return
			case "OFF", "TURN_OFF", "0", "FALSE":
				powerTrigger <- false
				fmt.Fprintf(conn, "OK: display turned off\n")
				return
			}
		}
	}
	fmt.Fprintf(conn, "ERR: unknown command\n")
}

func defaultSocketPath() string {
	if runtime.GOOS == "linux" {
		return "/run/securityhub-display.sock"
	}
	return filepath.Join(os.TempDir(), "securityhub-display.sock")
}

func detectOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return ""
}

func startMockStatusServer() string {
	startTime := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		isSetup := time.Since(startTime) < 12*time.Second
		fmt.Fprintf(w, `{"setup_required": %t, "connected_devices": 3, "ip_address": "192.168.10.1", "turn_on": true}`, isSetup)
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
