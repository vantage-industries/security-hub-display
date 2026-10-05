package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"security-hub-display/pkg/driver"
	"security-hub-display/pkg/ui"
)

func main() {
	var (
		busPath    = flag.String("bus", "/dev/i2c-1", "Path to I2C device (e.g. /dev/i2c-1)")
		addrStr    = flag.String("addr", "0x3C", "I2C address in hex (0x3C or 0x3D)")
		colOffset  = flag.Int("offset", 2, "SH1106 column RAM offset (MOD-08867 is typically 2)")
		loop       = flag.Bool("loop", true, "Keep running with a live heartbeat indicator")
		termMode   = flag.Bool("term", false, "Render preview in terminal instead of physical I2C")
	)
	flag.Parse()

	log.Println("=== MOD-08867 OLED Hello World ===")
	log.Printf("Target architecture: %s/%s", runtime.GOOS, runtime.GOARCH)

	if runtime.GOOS != "linux" && !*termMode {
		log.Printf("Running on %s (non-Linux). Auto-enabling --term terminal preview mode.", runtime.GOOS)
		*termMode = true
	}

	addr, err := parseHexOrDec(*addrStr)
	if err != nil {
		log.Fatalf("Invalid I2C address %q: %v", *addrStr, err)
	}

	var disp driver.DisplayDevice
	if *termMode {
		log.Println("Using terminal ANSI preview driver.")
		disp = driver.NewTerminalDisplay(true)
	} else {
		checkI2CDevices(*busPath)

		log.Printf("Connecting to OLED on %s at address 0x%02X (col offset %d)...", *busPath, addr, *colOffset)

		hwDisp, err := driver.NewSH1106(driver.SH1106Options{
			DevPath:      *busPath,
			Address:      byte(addr),
			ColumnOffset: byte(*colOffset),
		})
		if err != nil {
			log.Fatalf("Driver creation failed: %v", err)
		}
		disp = hwDisp
	}
	defer disp.Close()

	if err := disp.Init(); err != nil {
		log.Fatalf("Initialization failed: %v\nCheck wiring (VCC=3.3V, GND, SCL=Pin 5, SDA=Pin 3) and I2C address.", err)
	}
	log.Println("Display initialized successfully!")

	img := image.NewRGBA(image.Rect(0, 0, 128, 64))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	heartbeat := false
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		for y := 0; y < 64; y++ {
			for x := 0; x < 128; x++ {
				img.Set(x, y, ui.ColorBlack)
			}
		}

		ui.DrawBorder(img)
		ui.DrawInvertedHeader(img, "MOD-08867 OLED")
		ui.DrawCenteredText(img, 30, "HELLO WORLD!", ui.ColorWhite)
		ui.DrawCenteredText(img, 44, "Yocto Embedded Linux", ui.ColorWhite)
		ui.DrawCenteredText(img, 56, fmt.Sprintf("%s @ 0x%02X", filepath.Base(*busPath), addr), ui.ColorWhite)

		if heartbeat {
			img.Set(124, 60, ui.ColorWhite)
			img.Set(125, 60, ui.ColorWhite)
			img.Set(124, 61, ui.ColorWhite)
			img.Set(125, 61, ui.ColorWhite)
		}

		if err := disp.Draw(img); err != nil {
			log.Printf("Draw error: %v", err)
		}

		if !*loop {
			log.Println("Frame drawn. Exiting (--loop=false).")
			return
		}

		select {
		case <-sigChan:
			log.Println("Stopping Hello World display...")
			return
		case <-ticker.C:
			heartbeat = !heartbeat
		}
	}
}

func checkI2CDevices(targetBus string) {
	if _, err := os.Stat(targetBus); os.IsNotExist(err) {
		log.Printf("WARNING: Device %s does not exist!", targetBus)
		matches, _ := filepath.Glob("/dev/i2c*")
		if len(matches) > 0 {
			log.Printf("Found other I2C devices: %s", strings.Join(matches, ", "))
		} else {
			log.Println("No /dev/i2c* devices found on this system.")
			log.Println("Tip for Yocto: Ensure 'modprobe i2c-dev' is loaded or 'dtparam=i2c_arm=on' is in config.txt.")
		}
	} else {
		log.Printf("Device node %s is present.", targetBus)
	}
}

func parseHexOrDec(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseUint(s[2:], 16, 8)
	}
	return strconv.ParseUint(s, 10, 8)
}
