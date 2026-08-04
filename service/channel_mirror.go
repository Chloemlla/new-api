package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// ChannelMirrorConfig holds the configuration for the traffic mirroring feature.
type ChannelMirrorConfig struct {
	Enabled      bool
	MirrorRatio  float64 // 0.0 - 1.0, percentage of requests to mirror
}

var (
	mirrorConfig ChannelMirrorConfig
	mirrorCount  int64
)

// GetChannelMirrorConfig returns the current mirror configuration.
func GetChannelMirrorConfig() ChannelMirrorConfig {
	return mirrorConfig
}

// SetChannelMirrorConfig updates the mirror configuration.
func SetChannelMirrorConfig(cfg ChannelMirrorConfig) {
	mirrorConfig = cfg
}

// maybeMirrorRequest mirrors a request to a secondary channel for testing.
// The mirror response is NOT returned to the client; it's only logged for
// comparison purposes. This allows operators to validate new channels
// before routing production traffic to them.
func MaybeMirrorRequest(c *gin.Context, originalChannel *model.Channel, relayInfo interface{}) {
	cfg := GetChannelMirrorConfig()
	if !cfg.Enabled || cfg.MirrorRatio <= 0 {
		return
	}

	// Sample based on ratio.
	count := atomic.AddInt64(&mirrorCount, 1)
	if float64(count%100)/100.0 > cfg.MirrorRatio {
		return
	}

	// Find a mirror channel (a channel of the same type that is not the original).
	mirrorChannel := selectMirrorChannel(originalChannel)
	if mirrorChannel == nil {
		return
	}

	// Launch the mirror request asynchronously.
	go doMirrorRequest(c.Request, mirrorChannel, originalChannel.Id)
}

// selectMirrorChannel finds a suitable mirror channel.
// It looks for an enabled channel of the same type (excluding the original).
// In a production setup, mirror targets would be configured per-channel
// via a "mirror_of" field in the channel settings.
func selectMirrorChannel(original *model.Channel) *model.Channel {
	enabled := model.GetAllEnabledChannels()
	for _, ch := range enabled {
		if ch.Id != original.Id && ch.Type == original.Type {
			return ch
		}
	}
	return nil
}

// doMirrorRequest sends a copy of the request to the mirror channel.
func doMirrorRequest(originalReq *http.Request, mirrorChannel *model.Channel, originalChannelID int) {
	// Read the request body.
	bodyBytes, err := io.ReadAll(originalReq.Body)
	if err != nil {
		logger.LogError(nil, fmt.Sprintf("mirror: failed to read request body: %v", err))
		return
	}
	originalReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Build a new request for the mirror channel.
	baseURL := ""
	if mirrorChannel.BaseURL != nil {
		baseURL = *mirrorChannel.BaseURL
	}
	mirrorURL := fmt.Sprintf("%s%s", baseURL, originalReq.URL.Path)

	mirrorReq, err := http.NewRequest(originalReq.Method, mirrorURL, bytes.NewReader(bodyBytes))
	if err != nil {
		logger.LogError(nil, fmt.Sprintf("mirror: failed to create mirror request: %v", err))
		return
	}

	// Copy headers.
	mirrorReq.Header = originalReq.Header.Clone()

	// Use the mirror channel's key.
	key, _, keyErr := mirrorChannel.GetNextEnabledKey()
	if keyErr == nil && key != "" {
		mirrorReq.Header.Set("Authorization", "Bearer "+key)
	}

	// Send the mirrored request with a timeout.
	client := &http.Client{Timeout: common.GetEnvOrDefaultDuration("MIRROR_REQUEST_TIMEOUT", 30)}
	resp, err := client.Do(mirrorReq)
	if err != nil {
		logger.LogError(nil, fmt.Sprintf("mirror: request to channel #%d failed: %v", mirrorChannel.Id, err))
		return
	}
	defer resp.Body.Close()

	// Log the mirror result.
	responseBody, _ := io.ReadAll(resp.Body)
	logger.LogInfo(nil, fmt.Sprintf("mirror: channel #%d -> #%d | status=%d | body_size=%d | path=%s",
		originalChannelID, mirrorChannel.Id, resp.StatusCode, len(responseBody), originalReq.URL.Path))
}