package observer

import "time"

type ServiceStatus string

const (
	StatusWaitingForLogs ServiceStatus = "WAITING"
	StatusSetupRequired  ServiceStatus = "SETUP_REQUIRED"
	StatusConfigured     ServiceStatus = "CONFIGURED"
	StatusError          ServiceStatus = "ERROR"
)

type SetupInfo struct {
	Token        string    `json:"token"`
	Endpoint     string    `json:"endpoint"`
	PayloadHint  string    `json:"payload_hint"`
	DiscoveredAt time.Time `json:"discovered_at"`
	RawBanner    string    `json:"raw_banner"`
}

type ServiceState struct {
	Status      ServiceStatus `json:"status"`
	Setup       *SetupInfo    `json:"setup,omitempty"`
	Message     string        `json:"message,omitempty"`
	LastUpdated time.Time     `json:"last_updated"`
}
