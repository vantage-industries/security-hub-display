# security-hub-display

**security-hub-display** is a Go-based observer and display controller designed to tail systemd journal logs (`journalctl -u security-hub-api`), detect bootstrap setup tokens, and render setup QR codes, tokens, and system status onto a **MOD-08867** (1.3" 128x64 I2C OLED display with SH1106 controller).

It is written in pure Go with **zero CGO dependencies (`CGO_ENABLED=0`)**, allowing trivial cross-compilation for Raspberry Pi / Linux SBCs (ARM64, ARMv7, AMD64).

---

## Features

- **Automatic Log Observation**: Continuously tails `journalctl -u security-hub-api -f` with auto-reconnect resilience. Also supports reading from `stdin` or synthetic `--mock` mode.
- **Status API Polling (`/setup/status`)**: Periodically queries `GET /setup/status` (or `/api/v1/setup/status`). Displays setup tokens and QR code **only** when `setup_required == true`. Once setup is complete (`setup_required == false`), automatically switches to the "System Active" screen.
- **Token Detection & Parsing**: Accurately extracts one-time bootstrap tokens and API endpoints from the `SecurityHub is not yet configured.` banner.
- **MOD-08867 / SH1106 OLED Driver**: Direct, pure Go Linux I2C ioctl communication (`/dev/i2c-1`) with column offset compensation for 132x64 RAM panels.
- **Dynamic 128x64 UI Carousel**:
  - **Page 1 (QR Code + Summary)**: Crisp 52x52 QR Code for instant mobile phone scanning.
  - **Page 2 (Full Token View)**: Displays the full 43-character token split across lines in clear high-contrast font.
  - **Page 3 (Bootstrap Endpoint)**: Displays `POST /api/v1/setup/bootstrap`.
  - **Configured / Active View**: Displays `SECURITY HUB: ACTIVE / OK` when initialized.
- **Development Simulators**:
  - **Terminal ANSI Visualizer**: Renders the 128x64 display into your terminal using Unicode half-blocks (`█`, `▀`, `▄`) with vibrant OLED blue color.
  - **Web Preview Simulator**: Built-in HTTP server (`:8085`) rendering a live browser UI of the OLED screen.
- **Power Management**: Configurable screen blanking timeout (`--dim-timeout`) to prevent OLED phosphor burn-in.

---

## Hardware Wiring (MOD-08867 / 1.3" I2C OLED)

Connect the MOD-08867 module to your Raspberry Pi GPIO header:

| MOD-08867 Pin | Raspberry Pi GPIO Pin | Description |
|---|---|---|
| **VCC** | Pin 1 (3.3V) | 3.3V Power |
| **GND** | Pin 6 / Pin 9 | Ground |
| **SCL** | Pin 5 (GPIO 3 / SCL) | I2C Clock |
| **SDA** | Pin 3 (GPIO 2 / SDA) | I2C Data |

### Enable I2C on Raspberry Pi
```bash
# Enable I2C via raspi-config
sudo raspi-config
# Navigate to: Interface Options -> I2C -> Enable -> Yes

# Or add to /boot/config.txt:
# dtparam=i2c_arm=on

# Verify the device is detected at address 0x3c:
sudo apt-get install -y i2c-tools
i2cdetect -y 1
```

---

## Building and Cross-Compiling

All builds use `CGO_ENABLED=0` for maximum portability.

```bash
# Build for current host platform
make build

# Cross-compile for Raspberry Pi 3/4/5 64-bit (Linux ARM64)
make build-linux-arm64

# Cross-compile for Raspberry Pi 2/3/Zero 2 32-bit (Linux ARMv7)
make build-linux-armv7

# Cross-compile for Linux x86_64
make build-linux-amd64
```

---

## Running

### 1. On Physical Hardware (Raspberry Pi / Linux SBC)
```bash
sudo ./bin/security-hub-display \
  --unit=security-hub-api \
  --status-url=http://127.0.0.1:8080/setup/status \
  --mode=i2c \
  --i2c-bus=/dev/i2c-1 \
  --i2c-addr=0x3C \
  --carousel=5
```

### 2. Local Testing with Terminal Preview
```bash
# Using synthetic mock token and simulated status API:
./bin/security-hub-display --mode=terminal --mock

# Piping raw logs from stdin:
cat << 'EOF' | ./bin/security-hub-display --stdin --mode=terminal
========================================================================
SecurityHub is not yet configured.

Complete setup with this one-time token:

    1blgB3dnXGFfHZJzQ9pbmWJU2c_ESdFSoJnLfgdS1Ak

    POST /api/v1/setup/bootstrap { setup_token, username, password, full_name }

This token is valid until used and is regenerated on every restart.
========================================================================
EOF
```

### 3. Local Testing with Web Preview
```bash
./bin/security-hub-display --mode=web --mock --web-port=8085
# Open http://localhost:8085 in your browser
```

---

## CLI Options & Environment Variables

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `--unit` | `SECURITY_HUB_UNIT` | `security-hub-api` | Systemd service unit to observe |
| `--status-url` | `STATUS_URL` | `http://127.0.0.1:8080/setup/status` | URL to `/setup/status` endpoint (empty to disable HTTP check) |
| `--poll-interval` | - | `2` | Poll interval in seconds for status endpoint |
| `--mode` | `DISPLAY_MODE` | `i2c` (Linux) / `terminal` (macOS) | Display mode: `i2c`, `terminal`, `web`, `all` |
| `--i2c-bus` | `I2C_BUS` | `/dev/i2c-1` | Linux I2C device bus path |
| `--i2c-addr` | `I2C_ADDR` | `0x3C` | I2C slave address |
| `--col-offset` | - | `2` | SH1106 column RAM offset (2 for SH1106, 0 for SSD1306) |
| `--carousel` | - | `5` | Carousel rotation interval in seconds |
| `--qr-prefix` | `QR_PREFIX` | `""` | Optional URL prefix for QR code (e.g. `http://security.local/setup?token=`) |
| `--dim-timeout` | - | `0` | OLED sleep timeout in seconds after activity (0 = always on) |
| `--web-port` | - | `8085` | Port for web simulator (`--mode=web` or `--mode=all`) |
| `--mock` | - | `false` | Generate synthetic setup token logs and mock API for testing |
| `--stdin` | - | `false` | Read logs from stdin instead of running journalctl |

---

## Systemd Service Installation

1. Copy the binary to `/opt/security-hub-display/`:
   ```bash
   sudo mkdir -p /opt/security-hub-display
   sudo cp bin/security-hub-display-linux-arm64 /opt/security-hub-display/security-hub-display
   sudo chmod +x /opt/security-hub-display/security-hub-display
   ```

2. Install and enable the systemd service:
   ```bash
   sudo cp security-hub-display.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now security-hub-display.service
   ```

3. Check logs:
   ```bash
   journalctl -u security-hub-display -f
   ```
