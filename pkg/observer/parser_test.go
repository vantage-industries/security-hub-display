package observer

import (
	"strings"
	"testing"
)

func TestParseSetupBanner(t *testing.T) {
	rawLogs := `========================================================================
SecurityHub is not yet configured.

Complete setup with this one-time token:

    1blgB3dnXGFfHZJzQ9pbmWJU2c_ESdFSoJnLfgdS1Ak

    POST /api/v1/setup/bootstrap { setup_token, username, password, full_name }

This token is valid until used and is regenerated on every restart.
========================================================================`

	parser := NewParser()
	lines := strings.Split(rawLogs, "\n")

	var lastState *ServiceState
	for _, line := range lines {
		if state := parser.ParseLine(line); state != nil {
			lastState = state
		}
	}

	if lastState == nil {
		t.Fatalf("expected non-nil ServiceState, got nil")
	}

	if lastState.Status != StatusSetupRequired {
		t.Errorf("expected StatusSetupRequired, got %v", lastState.Status)
	}

	if lastState.Setup == nil {
		t.Fatalf("expected SetupInfo, got nil")
	}

	expectedToken := "1blgB3dnXGFfHZJzQ9pbmWJU2c_ESdFSoJnLfgdS1Ak"
	if lastState.Setup.Token != expectedToken {
		t.Errorf("expected token %q, got %q", expectedToken, lastState.Setup.Token)
	}

	expectedEndpoint := "POST /api/v1/setup/bootstrap"
	if lastState.Setup.Endpoint != expectedEndpoint {
		t.Errorf("expected endpoint %q, got %q", expectedEndpoint, lastState.Setup.Endpoint)
	}
}

func TestParseConfiguredTransition(t *testing.T) {
	parser := NewParser()

	lines := []string{
		"SecurityHub is not yet configured.",
		"Complete setup with this one-time token:",
		"    abc123XYZ456_testToken78901234567890",
		"    POST /api/v1/setup/bootstrap",
	}
	for _, line := range lines {
		parser.ParseLine(line)
	}

	state := parser.ParseLine("2026-08-17 12:00:00 [INFO] Setup completed successfully. SecurityHub is configured and listening.")
	if state == nil {
		t.Fatalf("expected state transition to configured, got nil")
	}
	if state.Status != StatusConfigured {
		t.Errorf("expected StatusConfigured, got %v", state.Status)
	}
}
