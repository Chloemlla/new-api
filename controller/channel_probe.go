package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// channelProbeTimeout bounds a single probe request so a hung upstream never
// blocks the probe loop.
const channelProbeTimeout = 10 * time.Second

// channelProbeDisableThreshold is the number of consecutive probe failures
// required before an unhealthy channel is auto-disabled. A single transient
// blip (e.g. a momentary 5xx on the model list endpoint) must not take a
// working channel offline.
const channelProbeDisableThreshold = 3

// channelProbeState classifies the outcome of one lightweight probe.
type channelProbeState string

const (
	// channelProbeHealthy means the upstream responded successfully to a
	// lightweight model list request: the base URL is reachable and the key
	// authenticates.
	channelProbeHealthy channelProbeState = "healthy"
	// channelProbeUnhealthy means the upstream could not be reached, rejected
	// the key, or failed the request: the channel is very likely unable to serve
	// real traffic.
	channelProbeUnhealthy channelProbeState = "unhealthy"
	// channelProbeInconclusive means health could not be determined from the
	// lightweight probe (e.g. the provider does not expose a model list
	// endpoint). Inconclusive probes never trigger auto-disable.
	channelProbeInconclusive channelProbeState = "inconclusive"
)

// channelProbeResult is the outcome of probing one channel.
type channelProbeResult struct {
	State          channelProbeState
	ResponseTimeMs int64
	Message        string
}

// channelProbeSummary records the outcome of one probe cycle so the system task
// can persist a per-run result for history.
type channelProbeSummary struct {
	Probed       int `json:"probed"`
	Healthy      int `json:"healthy"`
	Unhealthy    int `json:"unhealthy"`
	Inconclusive int `json:"inconclusive"`
	Disabled     int `json:"disabled"`
}

// channelProbeFailures tracks consecutive probe failures per channel so a
// single transient blip does not auto-disable a channel. Process-local state is
// sufficient because the channel_probe system task is executed by exactly one
// runner at a time (DB lease dedup).
var channelProbeFailures sync.Map // channelID (int) -> consecutive failure count (int)

// channelProbeUnsupportedTypes lists channel types whose health cannot be
// verified with a lightweight model list request (task-based platforms or
// OAuth-key flows); their health is only verifiable through a full channel
// test, so probes report them as inconclusive.
var channelProbeUnsupportedTypes = []int{
	constant.ChannelTypeMidjourney,
	constant.ChannelTypeMidjourneyPlus,
	constant.ChannelTypeSunoAPI,
	constant.ChannelTypeKling,
	constant.ChannelTypeJimeng,
	constant.ChannelTypeDoubaoVideo,
	constant.ChannelTypeVidu,
	constant.ChannelTypeCodex,
}

func isChannelProbeUnsupported(channelType int) bool {
	for _, unsupported := range channelProbeUnsupportedTypes {
		if channelType == unsupported {
			return true
		}
	}
	return false
}

// openAICompatibleModelListURL returns the model list endpoint URL for the
// standard OpenAI-compatible channel types. It is shared by upstream model
// discovery and channel health probes so both always probe the same real
// endpoint the provider exposes.
func openAICompatibleModelListURL(baseURL string, channelType int) string {
	switch channelType {
	case constant.ChannelTypeAli:
		return fmt.Sprintf("%s/compatible-mode/v1/models", baseURL)
	case constant.ChannelTypeZhipu_v4:
		if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/models", plan.OpenAIBaseURL)
		}
		return fmt.Sprintf("%s/api/paas/v4/models", baseURL)
	case constant.ChannelTypeVolcEngine:
		if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/v1/models", plan.OpenAIBaseURL)
		}
		return fmt.Sprintf("%s/v1/models", baseURL)
	case constant.ChannelTypeMoonshot:
		if plan, ok := constant.ChannelSpecialBases[baseURL]; ok && plan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/models", plan.OpenAIBaseURL)
		}
		return fmt.Sprintf("%s/v1/models", baseURL)
	default:
		return fmt.Sprintf("%s/v1/models", baseURL)
	}
}

// channelProbeURL returns the lightweight endpoint used to probe a channel's
// health. It reuses the same provider-specific model list paths as upstream
// model discovery so the probe verifies reachability and key validity on an
// endpoint the provider actually exposes.
func channelProbeURL(channel *model.Channel, baseURL string) (string, error) {
	switch channel.Type {
	case constant.ChannelTypeOllama:
		return fmt.Sprintf("%s/api/tags", baseURL), nil
	case constant.ChannelTypeGemini:
		return fmt.Sprintf("%s/v1beta/models", baseURL), nil
	default:
		return openAICompatibleModelListURL(baseURL, channel.Type), nil
	}
}

// buildChannelProbeRequest constructs the lightweight GET request used to probe
// a channel, reusing the same per-channel key resolution, headers, and header
// overrides as upstream model discovery so the probe reflects how real relay
// requests are authenticated.
func buildChannelProbeRequest(channel *model.Channel) (*http.Request, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, fmt.Errorf("failed to resolve channel key: %s", apiErr.Error())
	}
	key = strings.TrimSpace(key)

	probeURL, err := channelProbeURL(channel, baseURL)
	if err != nil {
		return nil, err
	}

	headers, err := buildFetchModelsHeaders(channel, key)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodGet, probeURL, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
		if strings.EqualFold(name, "Host") {
			request.Host = headers.Get(name)
		}
	}
	return request, nil
}

// probeChannelHealth runs a lightweight model list request against the channel
// and classifies the outcome. It never reveals the channel key in the returned
// message.
func probeChannelHealth(ctx context.Context, channel *model.Channel) channelProbeResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if channel == nil {
		return channelProbeResult{State: channelProbeInconclusive, Message: "channel is nil"}
	}
	if isChannelProbeUnsupported(channel.Type) {
		return channelProbeResult{
			State:   channelProbeInconclusive,
			Message: fmt.Sprintf("%s channels are not supported by channel probes", constant.GetChannelTypeName(channel.Type)),
		}
	}

	request, err := buildChannelProbeRequest(channel)
	if err != nil {
		return channelProbeResult{
			State:   channelProbeInconclusive,
			Message: sanitizeFetchModelsError(err, channel.Key).Error(),
		}
	}

	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return channelProbeResult{
			State:   channelProbeUnhealthy,
			Message: sanitizeFetchModelsError(err, channel.Key).Error(),
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, channelProbeTimeout)
	defer cancel()

	start := time.Now()
	response, err := client.Do(request.WithContext(probeCtx))
	responseTimeMs := time.Since(start).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return channelProbeResult{
				State:          channelProbeUnhealthy,
				ResponseTimeMs: responseTimeMs,
				Message:        "probe timed out",
			}
		}
		return channelProbeResult{
			State:          channelProbeUnhealthy,
			ResponseTimeMs: responseTimeMs,
			Message:        sanitizeFetchModelsError(err, channel.Key).Error(),
		}
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		return channelProbeResult{State: channelProbeHealthy, ResponseTimeMs: responseTimeMs}
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return channelProbeResult{
			State:          channelProbeUnhealthy,
			ResponseTimeMs: responseTimeMs,
			Message:        fmt.Sprintf("authentication failed: %s", response.Status),
		}
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed:
		// The provider does not expose a model list endpoint, so health cannot
		// be determined from this probe.
		return channelProbeResult{
			State:          channelProbeInconclusive,
			ResponseTimeMs: responseTimeMs,
			Message:        fmt.Sprintf("model list endpoint not available: %s", response.Status),
		}
	default:
		return channelProbeResult{
			State:          channelProbeUnhealthy,
			ResponseTimeMs: responseTimeMs,
			Message:        fmt.Sprintf("upstream returned %s", response.Status),
		}
	}
}

// maybeAutoDisableChannelByProbe counts consecutive probe failures and, once the
// threshold is reached, auto-disables the channel subject to the global
// AutomaticDisableChannelEnabled switch and the channel's own auto-ban flag. A
// healthy probe resets the counter; an inconclusive probe leaves it untouched.
func maybeAutoDisableChannelByProbe(channel *model.Channel, reason string) {
	failures := 0
	if value, ok := channelProbeFailures.Load(channel.Id); ok {
		failures, _ = value.(int)
	}
	failures++
	if failures < channelProbeDisableThreshold {
		channelProbeFailures.Store(channel.Id, failures)
		return
	}
	channelProbeFailures.Delete(channel.Id)

	if !common.AutomaticDisableChannelEnabled || !channel.GetAutoBan() {
		return
	}
	channelError := types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan())
	service.DisableChannel(*channelError, "channel probe: "+reason)
	common.SysLog(fmt.Sprintf(
		"channel #%d (%s) disabled by probe after %d consecutive failures: %s",
		channel.Id,
		channel.Name,
		channelProbeDisableThreshold,
		reason,
	))
}

// performChannelProbes runs the channel probe loop synchronously, honoring ctx
// cancellation so a system-task runner that loses its lease stops promptly.
// When report is non-nil it is called after each channel with (processed, total)
// so the system task can surface progress.
func performChannelProbes(ctx context.Context, channels []*model.Channel, report func(processed, total int)) channelProbeSummary {
	summary := channelProbeSummary{}
	total := len(channels)
	for index, channel := range channels {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if report != nil {
			report(index, total)
		}
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		summary.Probed++

		result := probeChannelHealth(ctx, channel)
		switch result.State {
		case channelProbeHealthy:
			summary.Healthy++
			channelProbeFailures.Delete(channel.Id)
		case channelProbeUnhealthy:
			summary.Unhealthy++
			maybeAutoDisableChannelByProbe(channel, result.Message)
		case channelProbeInconclusive:
			summary.Inconclusive++
		}

		if common.RequestInterval > 0 {
			if ctx == nil {
				time.Sleep(common.RequestInterval)
			} else {
				select {
				case <-ctx.Done():
					return summary
				case <-time.After(common.RequestInterval):
				}
			}
		}
	}
	if report != nil && (ctx == nil || ctx.Err() == nil) {
		report(total, total) // mark complete only when the full set was probed
	}
	return summary
}

// runChannelProbeTask runs one synchronous channel probe cycle for the system
// task runner (both the scheduled job and the manual "probe all channels"
// trigger go through here). It honors ctx cancellation so a runner that loses
// its lease stops promptly.
func runChannelProbeTask(ctx context.Context, report func(processed, total int)) (channelProbeSummary, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return channelProbeSummary{}, err
	}
	return performChannelProbes(ctx, channels, report), nil
}

// ProbeChannel runs a one-off lightweight health probe against a single channel
// without consuming any generation quota.
func ProbeChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	requestCtx := context.Background()
	if c.Request != nil {
		requestCtx = c.Request.Context()
	}
	result := probeChannelHealth(requestCtx, channel)
	c.JSON(http.StatusOK, gin.H{
		"success": result.State == channelProbeHealthy,
		"state":   result.State,
		"message": result.Message,
		"time":    float64(result.ResponseTimeMs) / 1000.0,
	})
}

// ProbeAllChannels enqueues a channel_probe system task instead of running the
// probe loop inline. If any channel_probe task is already active, the manual run
// is rejected so the caller does not mistake a scheduled run for this manual one.
func ProbeAllChannels(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelProbe, nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "已有通道健康检查任务正在运行或等待中，不能启动本次手动任务",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
				"type":    task.Type,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"task_id": task.TaskID,
			"status":  task.Status,
		},
	})
}