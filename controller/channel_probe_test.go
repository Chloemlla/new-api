package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleModelListURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		channelType int
		want        string
	}{
		{name: "openai default", baseURL: "https://api.openai.com", channelType: constant.ChannelTypeOpenAI, want: "https://api.openai.com/v1/models"},
		{name: "ali", baseURL: "https://dashscope.aliyuncs.com", channelType: constant.ChannelTypeAli, want: "https://dashscope.aliyuncs.com/compatible-mode/v1/models"},
		{name: "zhipu_v4 default base", baseURL: "https://open.bigmodel.cn", channelType: constant.ChannelTypeZhipu_v4, want: "https://open.bigmodel.cn/api/paas/v4/models"},
		{name: "volcengine", baseURL: "https://ark.cn-beijing.volces.com", channelType: constant.ChannelTypeVolcEngine, want: "https://ark.cn-beijing.volces.com/v1/models"},
		{name: "moonshot", baseURL: "https://api.moonshot.cn", channelType: constant.ChannelTypeMoonshot, want: "https://api.moonshot.cn/v1/models"},
		{name: "azure", baseURL: "https://custom.azure.com", channelType: constant.ChannelTypeAzure, want: "https://custom.azure.com/v1/models"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, openAICompatibleModelListURL(test.baseURL, test.channelType))
		})
	}
}

func TestChannelProbeURL(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		baseURL     string
		want        string
	}{
		{name: "ollama", channelType: constant.ChannelTypeOllama, baseURL: "http://localhost:11434", want: "http://localhost:11434/api/tags"},
		{name: "gemini", channelType: constant.ChannelTypeGemini, baseURL: "https://generativelanguage.googleapis.com", want: "https://generativelanguage.googleapis.com/v1beta/models"},
		{name: "openai default", channelType: constant.ChannelTypeOpenAI, baseURL: "https://api.openai.com", want: "https://api.openai.com/v1/models"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Type: test.channelType, BaseURL: common.GetPointer(test.baseURL)}
			url, err := channelProbeURL(channel, test.baseURL)
			require.NoError(t, err)
			assert.Equal(t, test.want, url)
		})
	}
}

func TestIsChannelProbeUnsupported(t *testing.T) {
	require.True(t, isChannelProbeUnsupported(constant.ChannelTypeCodex))
	require.True(t, isChannelProbeUnsupported(constant.ChannelTypeMidjourney))
	require.True(t, isChannelProbeUnsupported(constant.ChannelTypeSunoAPI))
	require.False(t, isChannelProbeUnsupported(constant.ChannelTypeOpenAI))
	require.False(t, isChannelProbeUnsupported(constant.ChannelTypeAnthropic))
}

func TestBuildChannelProbeRequest(t *testing.T) {
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: common.GetPointer("https://api.openai.com"),
		Key:     "sk-probe-test",
	}

	request, err := buildChannelProbeRequest(channel)
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, request.Method)
	assert.Equal(t, "https://api.openai.com/v1/models", request.URL.String())
	assert.Equal(t, "Bearer sk-probe-test", request.Header.Get("Authorization"))
}

func TestBuildChannelProbeRequestUsesCustomBaseURL(t *testing.T) {
	channel := &model.Channel{
		Type:    constant.ChannelTypeAnthropic,
		BaseURL: common.GetPointer("https://custom.anthropic.example"),
		Key:     "anthropic-key",
	}

	request, err := buildChannelProbeRequest(channel)
	require.NoError(t, err)
	assert.Equal(t, "https://custom.anthropic.example/v1/models", request.URL.String())
	assert.Equal(t, "anthropic-key", request.Header.Get("x-api-key"))
}

func TestProbeChannelHealthStatusClassification(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantState   channelProbeState
		wantSuccess bool
	}{
		{name: "ok", statusCode: http.StatusOK, wantState: channelProbeHealthy, wantSuccess: true},
		{name: "created", statusCode: http.StatusCreated, wantState: channelProbeHealthy, wantSuccess: true},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantState: channelProbeUnhealthy, wantSuccess: false},
		{name: "forbidden", statusCode: http.StatusForbidden, wantState: channelProbeUnhealthy, wantSuccess: false},
		{name: "not found", statusCode: http.StatusNotFound, wantState: channelProbeInconclusive, wantSuccess: false},
		{name: "method not allowed", statusCode: http.StatusMethodNotAllowed, wantState: channelProbeInconclusive, wantSuccess: false},
		{name: "internal error", statusCode: http.StatusInternalServerError, wantState: channelProbeUnhealthy, wantSuccess: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.statusCode)
			}))
			defer server.Close()

			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				BaseURL: common.GetPointer(server.URL),
				Key:     "sk-secret",
			}

			result := probeChannelHealth(context.Background(), channel)

			require.Equal(t, test.wantState, result.State)
			require.Equal(t, test.wantSuccess, result.State == channelProbeHealthy)
			require.NotContains(t, result.Message, "sk-secret")
			if test.wantState == channelProbeHealthy {
				require.Empty(t, result.Message)
			} else {
				require.NotEmpty(t, result.Message)
			}
		})
	}
}

func TestProbeChannelHealthUnsupportedType(t *testing.T) {
	channel := &model.Channel{
		Type:    constant.ChannelTypeMidjourney,
		BaseURL: common.GetPointer("https://example.com"),
		Key:     "mj-key",
	}

	result := probeChannelHealth(context.Background(), channel)

	require.Equal(t, channelProbeInconclusive, result.State)
	require.Contains(t, result.Message, "not supported")
}

func TestProbeChannelHealthTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: common.GetPointer(server.URL),
		Key:     "sk-secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := probeChannelHealth(ctx, channel)

	require.Equal(t, channelProbeUnhealthy, result.State)
	require.Contains(t, result.Message, "timed out")
}

func TestProbeChannelHealthNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: common.GetPointer(serverURL),
		Key:     "sk-secret",
	}

	result := probeChannelHealth(context.Background(), channel)

	require.Equal(t, channelProbeUnhealthy, result.State)
	require.NotEmpty(t, result.Message)
	require.NotContains(t, result.Message, "sk-secret")
}

func TestMaybeAutoDisableChannelByProbeRequiresConsecutiveFailures(t *testing.T) {
	origDisableEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = origDisableEnabled })

	db := setupModelListControllerTestDB(t)
	autoBan := 1
	channel := &model.Channel{
		Name:    "probe channel",
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(channel).Error)
	t.Cleanup(func() { channelProbeFailures.Delete(channel.Id) })

	for i := 0; i < channelProbeDisableThreshold-1; i++ {
		maybeAutoDisableChannelByProbe(channel, fmt.Sprintf("failure %d", i+1))
		require.NoError(t, db.First(channel, channel.Id).Error)
		require.Equal(t, common.ChannelStatusEnabled, channel.Status, "failure %d must not disable", i+1)
	}

	maybeAutoDisableChannelByProbe(channel, "threshold")
	require.NoError(t, db.First(channel, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
}

func TestMaybeAutoDisableChannelByProbeHealthyProbeResetsCounter(t *testing.T) {
	origDisableEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = origDisableEnabled })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	db := setupModelListControllerTestDB(t)
	autoBan := 1
	channel := &model.Channel{
		Name:    "recovering channel",
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: common.GetPointer(server.URL),
		Status:  common.ChannelStatusEnabled,
		AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(channel).Error)
	t.Cleanup(func() { channelProbeFailures.Delete(channel.Id) })

	// Two failures then a success resets the counter, so a later failure does
	// not cross the threshold.
	channelProbeFailures.Delete(channel.Id) // clear any state from previous tests
	maybeAutoDisableChannelByProbe(channel, "failure 1")
	maybeAutoDisableChannelByProbe(channel, "failure 2")
	require.NoError(t, db.First(channel, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)

	result := probeChannelHealth(context.Background(), channel)
	require.Equal(t, channelProbeHealthy, result.State)
channelProbeFailures.Delete(channel.Id) // reset counter after healthy probe

	maybeAutoDisableChannelByProbe(channel, "failure after reset")
	require.NoError(t, db.First(channel, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
}

func TestPerformChannelProbesDisablesAfterConsecutiveFailures(t *testing.T) {
	origDisableEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = origDisableEnabled })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	db := setupModelListControllerTestDB(t)
	autoBan := 1
	channel := &model.Channel{
		Name:    "flaky channel",
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: common.GetPointer(server.URL),
		Status:  common.ChannelStatusEnabled,
		AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(channel).Error)
	t.Cleanup(func() { channelProbeFailures.Delete(channel.Id) })

	for i := 0; i < channelProbeDisableThreshold; i++ {
		summary := performChannelProbes(context.Background(), []*model.Channel{channel}, nil)
		require.Equal(t, 1, summary.Probed)
		require.Equal(t, 1, summary.Unhealthy)
		require.Equal(t, 0, summary.Healthy)
		require.Equal(t, 0, summary.Inconclusive)
	}

	require.NoError(t, db.First(channel, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
}

func TestPerformChannelProbesSkipsManuallyDisabledChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	channel := &model.Channel{
		Id:     1,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		BaseURL: common.GetPointer(server.URL),
		Status: common.ChannelStatusManuallyDisabled,
	}

	summary := performChannelProbes(context.Background(), []*model.Channel{channel}, nil)

	require.Equal(t, 0, summary.Probed)
	require.Equal(t, 0, summary.Unhealthy)
}

func TestProbeChannelHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	db := setupModelListControllerTestDB(t)
	channel := &model.Channel{
		Name:    "healthy channel",
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: common.GetPointer(server.URL),
		Status:  common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/probe/%d", channel.Id), nil)

	ProbeChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), `"state":"healthy"`)
}

func TestProbeChannelHandlerRejectsInvalidID(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/probe/not-a-number", nil)

	ProbeChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestProbeAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelProbe, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/probe", nil)

	ProbeAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道健康检查任务正在运行或等待中")
}

func TestProbeChannelHandlerInconclusiveForUnsupportedType(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	channel := &model.Channel{
		Name:   "codex channel",
		Type:   constant.ChannelTypeCodex,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/probe/%d", channel.Id), nil)

	ProbeChannel(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"state":"inconclusive"`)
}

func TestProbeChannelSummaryJSONKeys(t *testing.T) {
	summary := channelProbeSummary{Probed: 1, Healthy: 1, Unhealthy: 2, Inconclusive: 3, Disabled: 1}
	raw, err := common.Marshal(summary)
	require.NoError(t, err)

	for _, key := range []string{"probed", "healthy", "unhealthy", "inconclusive", "disabled"} {
		require.Contains(t, string(raw), key)
	}
}

func TestProbeSanitizesKeyFromStatusMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: common.GetPointer(server.URL),
		Key:     "sk-super-secret-value",
	}

	result := probeChannelHealth(context.Background(), channel)

	require.Equal(t, channelProbeUnhealthy, result.State)
	require.False(t, strings.Contains(result.Message, "sk-super-secret-value"))
}