package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupLogExportTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "east"}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: 1, Username: "alice", Type: model.LogTypeConsume,
		ModelName: "gpt-a", TokenName: "primary", Quota: 100,
		PromptTokens: 10, CompletionTokens: 5, UseTime: 12,
		ChannelId: 1, TokenId: 11, Group: "default", Ip: "1.2.3.4",
		RequestId: "req-1", CreatedAt: 1100,
		Other: `{"admin_info":{"version":"1.0"},"model_ratio":1.0}`,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: 2, Username: "bob", Type: model.LogTypeConsume,
		ModelName: "gpt-b", TokenName: "backup", Quota: 70,
		PromptTokens: 20, CompletionTokens: 10, UseTime: 8,
		ChannelId: 1, TokenId: 22, Group: "vip", Ip: "5.6.7.8",
		RequestId: "req-2", CreatedAt: 1200,
		Other: `{"admin_info":{"version":"1.0"}}`,
	}).Error)
}

func TestExportAllLogsCSVIncludesAdminFieldsAndChannelNames(t *testing.T) {
	setupLogExportTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/export?format=csv", nil)

	ExportAllLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Disposition"), "attachment")
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")

	body := recorder.Body.String()
	require.Contains(t, body, "id,user_id,created_at")
	require.Contains(t, body, "east")
	require.Contains(t, body, "bob")
	require.Contains(t, body, "req-2")
	require.Contains(t, body, "gpt-b")
	// Admin export keeps admin-only fields so operators can audit raw data.
	require.Contains(t, body, "admin_info")
}

func TestExportUserLogsJSONStripsAdminFields(t *testing.T) {
	setupLogExportTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/export/self?format=json", nil)

	ExportUserLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

	var logs []*model.Log
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &logs))
	require.Len(t, logs, 1)
	require.Equal(t, 1, logs[0].UserId)
	require.Equal(t, "alice", logs[0].Username)
	require.NotContains(t, logs[0].Other, "admin_info")
	require.NotContains(t, logs[0].Other, "audit_info")
	require.Contains(t, logs[0].Other, "model_ratio")
}

func TestExportUserLogsIgnoresUsernameParam(t *testing.T) {
	setupLogExportTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	// A username param must not widen the user's own export to other users.
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/export/self?format=json&username=bob", nil)

	ExportUserLogs(ctx)

	var logs []*model.Log
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &logs))
	require.Len(t, logs, 1)
	require.Equal(t, "alice", logs[0].Username)
}

func TestExportAllLogsAppliesTimeRangeFilter(t *testing.T) {
	setupLogExportTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/export?format=json&start_timestamp=1150&end_timestamp=2000", nil)

	ExportAllLogs(ctx)

	var logs []*model.Log
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &logs))
	require.Len(t, logs, 1)
	require.Equal(t, "bob", logs[0].Username)
}

func TestExportLogsRejectsInvalidFormat(t *testing.T) {
	setupLogExportTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/export?format=xml", nil)

	ExportAllLogs(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.False(t, payload.Success)
	require.Equal(t, "invalid format", payload.Message)
	require.NotContains(t, recorder.Header().Get("Content-Disposition"), "attachment")
}

func TestExportAllLogsCSVDefaultsToCSVFormat(t *testing.T) {
	setupLogExportTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/export", nil)

	ExportAllLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/csv")
	// The CSV export is prefixed with a UTF-8 BOM for spreadsheet compatibility.
	require.True(t, strings.HasPrefix(recorder.Body.String(), "\xEF\xBB\xBF"))
}
