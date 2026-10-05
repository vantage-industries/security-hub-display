package observer

import (
	"regexp"
	"strings"
	"time"
)

var (
	reNotConfigured = regexp.MustCompile(`(?i)SecurityHub is not yet configured`)
	reTokenPrompt   = regexp.MustCompile(`(?i)Complete setup with this one-time token:`)
	reToken         = regexp.MustCompile(`^[A-Za-z0-9_\-]{20,80}$`)
	reEndpoint      = regexp.MustCompile(`(?i)(POST|GET|PUT)\s+([/\w\-]+(?:/[/\w\-]+)*)\s*(\{.*\})?`)
	reBannerBorder  = regexp.MustCompile(`^={20,}`)
	reConfigured    = regexp.MustCompile(`(?i)(SecurityHub is configured|Setup completed successfully|Bootstrap complete|Admin user created|SecurityHub started successfully)`)
)

type Parser struct {
	inBanner     bool
	expectToken  bool
	currentInfo  *SetupInfo
	bannerLines  []string
	lastToken    string
	lastEndpoint string
}

func NewParser() *Parser {
	return &Parser{
		bannerLines: make([]string, 0, 16),
	}
}

func (p *Parser) ParseLine(line string) *ServiceState {
	trimmed := strings.TrimSpace(line)

	if reConfigured.MatchString(trimmed) {
		p.inBanner = false
		p.expectToken = false
		p.bannerLines = p.bannerLines[:0]
		return &ServiceState{
			Status:      StatusConfigured,
			Message:     "SecurityHub is configured and active",
			LastUpdated: time.Now(),
		}
	}

	if reNotConfigured.MatchString(trimmed) {
		p.inBanner = true
		p.bannerLines = []string{line}
		p.currentInfo = &SetupInfo{
			DiscoveredAt: time.Now(),
		}
		return nil
	}

	if reTokenPrompt.MatchString(trimmed) {
		p.inBanner = true
		p.expectToken = true
		if p.currentInfo == nil {
			p.currentInfo = &SetupInfo{
				DiscoveredAt: time.Now(),
			}
		}
		p.bannerLines = append(p.bannerLines, line)
		return nil
	}

	if p.expectToken && trimmed != "" {
		cleanToken := strings.Trim(trimmed, "\"'`")
		if reToken.MatchString(cleanToken) {
			p.expectToken = false
			p.lastToken = cleanToken
			if p.currentInfo == nil {
				p.currentInfo = &SetupInfo{
					DiscoveredAt: time.Now(),
				}
			}
			p.currentInfo.Token = cleanToken
			p.bannerLines = append(p.bannerLines, line)

			return &ServiceState{
				Status:      StatusSetupRequired,
				Setup:       p.cloneInfo(p.currentInfo),
				Message:     "Setup required: token detected",
				LastUpdated: time.Now(),
			}
		}
	}

	if matches := reEndpoint.FindStringSubmatch(trimmed); len(matches) >= 3 {
		endpoint := matches[1] + " " + matches[2]
		var payloadHint string
		if len(matches) >= 4 {
			payloadHint = strings.TrimSpace(matches[3])
		}
		p.lastEndpoint = endpoint
		if p.currentInfo != nil {
			p.currentInfo.Endpoint = endpoint
			p.currentInfo.PayloadHint = payloadHint
			p.bannerLines = append(p.bannerLines, line)

			if p.currentInfo.Token != "" {
				p.currentInfo.RawBanner = strings.Join(p.bannerLines, "\n")
				return &ServiceState{
					Status:      StatusSetupRequired,
					Setup:       p.cloneInfo(p.currentInfo),
					Message:     "Setup required: token & endpoint ready",
					LastUpdated: time.Now(),
				}
			}
		}
	}

	if !p.inBanner && reToken.MatchString(trimmed) && len(trimmed) >= 32 {
		p.lastToken = trimmed
		info := &SetupInfo{
			Token:        trimmed,
			Endpoint:     p.lastEndpoint,
			DiscoveredAt: time.Now(),
			RawBanner:    trimmed,
		}
		return &ServiceState{
			Status:      StatusSetupRequired,
			Setup:       info,
			Message:     "Setup token detected",
			LastUpdated: time.Now(),
		}
	}

	if p.inBanner {
		p.bannerLines = append(p.bannerLines, line)
		if reBannerBorder.MatchString(trimmed) && p.currentInfo != nil && p.currentInfo.Token != "" {
			p.currentInfo.RawBanner = strings.Join(p.bannerLines, "\n")
			p.inBanner = false
			return &ServiceState{
				Status:      StatusSetupRequired,
				Setup:       p.cloneInfo(p.currentInfo),
				Message:     "Setup required",
				LastUpdated: time.Now(),
			}
		}
	}

	return nil
}

func (p *Parser) cloneInfo(info *SetupInfo) *SetupInfo {
	if info == nil {
		return nil
	}
	cp := *info
	return &cp
}
