package observer

import "time"

type ServiceStatus string

const (
	StatusWaitingForLogs ServiceStatus = "WAITING"
	StatusSetupRequired  ServiceStatus = "SETUP_REQUIRED"
	StatusConfigured     ServiceStatus = "CONFIGURED"
	StatusTest           ServiceStatus = "TEST"
	StatusError          ServiceStatus = "ERROR"
)

type SetupInfo struct {
	Token        string    `json:"token"`
	Endpoint     string    `json:"endpoint"`
	PayloadHint  string    `json:"payload_hint"`
	DiscoveredAt time.Time `json:"discovered_at"`
	RawBanner    string    `json:"raw_banner"`
}

type TestInfo struct {
	ActiveUntil time.Time `json:"active_until"`
	DeviceBus   string    `json:"device_bus,omitempty"`
	DeviceAddr  string    `json:"device_addr,omitempty"`
	IPAddress   string    `json:"ip_address,omitempty"`
	Message     string    `json:"message,omitempty"`
}

type ConfiguredInfo struct {
	IPAddress        string `json:"ip_address,omitempty"`
	ConnectedDevices int64  `json:"connected_devices"`
	TurnOn           bool   `json:"turn_on"`
}

type ServiceState struct {
	Status      ServiceStatus   `json:"status"`
	Setup       *SetupInfo      `json:"setup,omitempty"`
	Test        *TestInfo       `json:"test,omitempty"`
	Configured  *ConfiguredInfo `json:"configured,omitempty"`
	Message     string          `json:"message,omitempty"`
	LastUpdated time.Time       `json:"last_updated"`
}
