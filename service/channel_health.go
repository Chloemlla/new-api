package service

import (
	"fmt"
	http0 "net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// ChannelHealthProbeConfig holds configuration for the health check probe.
type ChannelHealthProbeConfig struct {
	Enabled         bool
	IntervalSeconds int
	TimeoutSeconds  int
	Concurrency     int
}

var (
	channelHealthProbeConfig ChannelHealthProbeConfig
	channelHealthMu          sync.Mutex
	channelHealthResults     sync.Map // map[int]*ChannelHealthResult
	channelHealthProbeOnce   sync.Once
)

// ChannelHealthResult stores the latest health check result for a channel.
type ChannelHealthResult struct {
	ChannelID   int       `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	LastChecked time.Time `json:"last_checked"`
	Success     bool      `json:"success"`
	LatencyMs   int64     `json:"latency_ms"`
	ErrorMsg    string    `json:"error_msg,omitempty"`
	ConsecutiveFailures int `json:"consecutive_failures"`
	StatusCode  int       `json:"status_code"`
}

// GetChannelHealthProbeConfig returns the current health probe configuration.
func GetChannelHealthProbeConfig() ChannelHealthProbeConfig {
	channelHealthMu.Lock()
	defer channelHealthMu.Unlock()
	return channelHealthProbeConfig
}

// SetChannelHealthProbeConfig updates the health probe configuration.
func SetChannelHealthProbeConfig(cfg ChannelHealthProbeConfig) {
	channelHealthMu.Lock()
	channelHealthProbeConfig = cfg
	channelHealthMu.Unlock()
}

// StartChannelHealthProbes begins periodic health checks for all enabled channels.
func StartChannelHealthProbes() {
	cfg := GetChannelHealthProbeConfig()
	if !cfg.Enabled || cfg.IntervalSeconds <= 0 {
		return
	}

	channelHealthProbeOnce.Do(func() {
		go func() {
			// Initial delay so the system can stabilize on startup.
			time.Sleep(30 * time.Second)
			for {
				runHealthProbes()
				cfg = GetChannelHealthProbeConfig()
				time.Sleep(time.Duration(cfg.IntervalSeconds) * time.Second)
			}
		}()
	})
}

// runHealthProbes checks all enabled channels.
func runHealthProbes() {
	cfg := GetChannelHealthProbeConfig()
	if !cfg.Enabled {
		return
	}

	channels := model.GetAllEnabledChannels()
	if len(channels) == 0 {
		return
	}

	// Use a semaphore to limit concurrency.
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for _, ch := range channels {
		ch := ch
		// Skip channels that don't have test models configured.
		if ch.TestModel == nil || *ch.TestModel == "" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			probeChannel(ch)
		}()
	}
	wg.Wait()
}

// probeChannel performs a single health check on a channel.
func probeChannel(channel *model.Channel) {
	start := time.Now()
	result := &ChannelHealthResult{
		ChannelID:   channel.Id,
		ChannelName: channel.Name,
		LastChecked: start,
	}

	// Build a minimal test request.
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = *channel.BaseURL
	}

	key, _, keyErr := channel.GetNextEnabledKey()
	if keyErr != nil || key == "" {
		result.Success = false
		result.ErrorMsg = "no available key"
		result.StatusCode = 0
		finalizeHealthResult(channel.Id, result)
		return
	}

	// Use the test model to check if the channel is responsive.
	client := &http0.Client{
		Timeout: time.Duration(GetChannelHealthProbeConfig().TimeoutSeconds) * time.Second,
	}

	// Build a test URL based on channel type.
	testURL := buildHealthCheckURL(channel, baseURL)
	if testURL == "" {
		// Skip channels that can't be easily probed (e.g., custom channels).
		result.Success = true
		result.LatencyMs = 0
		finalizeHealthResult(channel.Id, result)
		return
	}

	req, err := http0.NewRequest("GET", testURL, nil)
	if err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("failed to build request: %v", err)
		finalizeHealthResult(channel.Id, result)
		return
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("request failed: %v", err)
		result.StatusCode = 0
	} else {
		defer resp.Body.Close()
		result.StatusCode = resp.StatusCode
		// A 200 or 401 means the channel is reachable (401 = auth rejected but endpoint is alive).
		if resp.StatusCode == http0.StatusOK || resp.StatusCode == http0.StatusUnauthorized {
			result.Success = true
		} else {
			result.Success = false
			result.ErrorMsg = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		}
	}

	finalizeHealthResult(channel.Id, result)
}

// buildHealthCheckURL constructs a probe URL for the given channel type.
func buildHealthCheckURL(channel *model.Channel, baseURL string) string {
	if baseURL == "" {
		return ""
	}
	// For OpenAI-compatible channels, probe the /v1/models endpoint.
	// For other channels, skip the probe (return empty).
	switch {
	case channel.Type >= 1 && channel.Type <= 59:
		return fmt.Sprintf("%s/v1/models", baseURL)
	default:
		return ""
	}
}

// finalizeHealthResult stores the health check result and potentially auto-disables the channel.
func finalizeHealthResult(channelID int, result *ChannelHealthResult) {
	// Get previous result to track consecutive failures.
	prevRaw, loaded := channelHealthResults.LoadOrStore(channelID, result)
	if loaded {
		prev := prevRaw.(*ChannelHealthResult)
		if !result.Success {
			if !prev.Success {
				result.ConsecutiveFailures = prev.ConsecutiveFailures + 1
			} else {
				result.ConsecutiveFailures = 1
			}
		} else {
			result.ConsecutiveFailures = 0
		}
	} else if !result.Success {
		// First failure for this channel.
		result.ConsecutiveFailures = 1
	}
	channelHealthResults.Store(channelID, result)

	// Auto-disable after 3 consecutive failures.
	if !result.Success && result.ConsecutiveFailures >= 3 {
		if ch, err := model.CacheGetChannel(channelID); err == nil && ch != nil && ch.Status == common.ChannelStatusEnabled {
			logger.LogError(nil, fmt.Sprintf("health probe: channel #%d (%s) failed %d consecutive probes, auto-disabling",
				channelID, result.ChannelName, result.ConsecutiveFailures))
			model.UpdateChannelStatus(channelID, "", common.ChannelStatusAutoDisabled,
				fmt.Sprintf("health probe: %d consecutive failures", result.ConsecutiveFailures))
		}
	}
}

// GetChannelHealthResult returns the latest health check result for a channel.
func GetChannelHealthResult(channelID int) *ChannelHealthResult {
	if v, ok := channelHealthResults.Load(channelID); ok {
		return v.(*ChannelHealthResult)
	}
	return nil
}

// GetAllChannelHealthResults returns all health check results.
func GetAllChannelHealthResults() map[int]*ChannelHealthResult {
	results := make(map[int]*ChannelHealthResult)
	channelHealthResults.Range(func(key, value interface{}) bool {
		results[key.(int)] = value.(*ChannelHealthResult)
		return true
	})
	return results
}

// GetUnhealthyChannels returns channels that have failed their last health check.
func GetUnhealthyChannels() []*ChannelHealthResult {
	var unhealthy []*ChannelHealthResult
	channelHealthResults.Range(func(key, value interface{}) bool {
		result := value.(*ChannelHealthResult)
		if !result.Success {
			unhealthy = append(unhealthy, result)
		}
		return true
	})
	return unhealthy
}