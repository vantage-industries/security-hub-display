package observer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

type Config struct {
	UnitName  string
	UseStdin  bool
	Mock      bool
	Reader    io.Reader
	PollLines int
}

type Observer struct {
	cfg     Config
	parser  *Parser
	mu      sync.RWMutex
	state   ServiceState
	updates chan ServiceState
	ctx     context.Context
	cancel  context.CancelFunc
}

func New(cfg Config) *Observer {
	if cfg.UnitName == "" {
		cfg.UnitName = "security-hub-api"
	}
	if cfg.PollLines == 0 {
		cfg.PollLines = 100
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Observer{
		cfg:     cfg,
		parser:  NewParser(),
		updates: make(chan ServiceState, 10),
		ctx:     ctx,
		cancel:  cancel,
		state: ServiceState{
			Status:      StatusWaitingForLogs,
			Message:     "Waiting for logs from " + cfg.UnitName,
			LastUpdated: time.Now(),
		},
	}
}

func (o *Observer) Start() {
	go o.run()
}

func (o *Observer) Stop() {
	o.cancel()
}

func (o *Observer) Updates() <-chan ServiceState {
	return o.updates
}

func (o *Observer) CurrentState() ServiceState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.state
}

func (o *Observer) setState(s ServiceState) {
	o.mu.Lock()
	o.state = s
	o.mu.Unlock()

	select {
	case o.updates <- s:
	default:
		select {
		case <-o.updates:
		default:
		}
		o.updates <- s
	}
}

func (o *Observer) run() {
	if o.cfg.Mock {
		o.runMock()
		return
	}

	if o.cfg.UseStdin && o.cfg.Reader != nil {
		o.scanReader(o.cfg.Reader)
		return
	}

	for {
		select {
		case <-o.ctx.Done():
			return
		default:
		}

		err := o.tailJournal()
		if err != nil {
			log.Printf("[observer] journalctl exited: %v (reconnecting in 3s...)", err)
		}

		select {
		case <-o.ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (o *Observer) tailJournal() error {
	args := []string{
		"-u", o.cfg.UnitName,
		"-f",
		"-n", fmt.Sprintf("%d", o.cfg.PollLines),
		"-o", "cat",
	}

	cmd := exec.CommandContext(o.ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start journalctl: %w", err)
	}

	scanErr := o.scanReader(stdout)
	_ = cmd.Wait()
	return scanErr
}

func (o *Observer) scanReader(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-o.ctx.Done():
			return o.ctx.Err()
		default:
		}

		line := scanner.Text()
		if newState := o.parser.ParseLine(line); newState != nil {
			o.setState(*newState)
		}
	}
	return scanner.Err()
}

func (o *Observer) runMock() {
	log.Println("[observer] Running in MOCK mode")
	time.Sleep(1 * time.Second)

	mockLines := []string{
		"========================================================================",
		"SecurityHub is not yet configured.",
		"",
		"Complete setup with this one-time token:",
		"",
		"    1blgB3dnXGFfHZJzQ9pbmWJU2c_ESdFSoJnLfgdS1Ak",
		"",
		"    POST /api/v1/setup/bootstrap { setup_token, username, password, full_name }",
		"",
		"This token is valid until used and is regenerated on every restart.",
		"========================================================================",
	}

	for _, line := range mockLines {
		if newState := o.parser.ParseLine(line); newState != nil {
			o.setState(*newState)
		}
		time.Sleep(50 * time.Millisecond)
	}

	<-o.ctx.Done()
}
