package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Response struct {
	SetupRequired bool `json:"setup_required"`
}

type Result struct {
	Available     bool
	SetupRequired bool
	StatusCode    int
	Error         error
	CheckedAt     time.Time
}

type Client struct {
	primaryURL  string
	fallbackURL string
	httpClient  *http.Client
}

func NewClient(urlStr string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if urlStr == "" {
		urlStr = "http://127.0.0.1:8080/setup/status"
	}

	var fallback string
	if strings.HasSuffix(urlStr, "/setup/status") && !strings.Contains(urlStr, "/api/v1/") {
		fallback = strings.Replace(urlStr, "/setup/status", "/api/v1/setup/status", 1)
	} else if strings.Contains(urlStr, "/api/v1/setup/status") {
		fallback = strings.Replace(urlStr, "/api/v1/setup/status", "/setup/status", 1)
	}

	return &Client{
		primaryURL:  urlStr,
		fallbackURL: fallback,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) SetTransport(rt http.RoundTripper) {
	c.httpClient.Transport = rt
}

func (c *Client) Check(ctx context.Context) Result {
	res := c.checkURL(ctx, c.primaryURL)
	if !res.Available && c.fallbackURL != "" && res.StatusCode == http.StatusNotFound {
		res = c.checkURL(ctx, c.fallbackURL)
	}
	return res
}

func (c *Client) checkURL(ctx context.Context, targetURL string) Result {
	now := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return Result{Available: false, Error: err, CheckedAt: now}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{Available: false, Error: err, CheckedAt: now}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{
			Available:  false,
			StatusCode: resp.StatusCode,
			Error:      fmt.Errorf("unexpected status code: %d", resp.StatusCode),
			CheckedAt:  now,
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{
			Available:  false,
			StatusCode: resp.StatusCode,
			Error:      fmt.Errorf("read error: %w", err),
			CheckedAt:  now,
		}
	}

	var parsed Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Result{
			Available:  false,
			StatusCode: resp.StatusCode,
			Error:      fmt.Errorf("invalid json: %w", err),
			CheckedAt:  now,
		}
	}

	return Result{
		Available:     true,
		SetupRequired: parsed.SetupRequired,
		StatusCode:    resp.StatusCode,
		CheckedAt:     now,
	}
}
