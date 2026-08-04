package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// logExportCSVHeader is the stable column schema of the CSV usage-log export.
// Column names match the JSON field names of the log records so both export
// formats stay consistent for downstream tooling.
var logExportCSVHeader = []string{
	"id", "user_id", "created_at", "type", "content", "username",
	"token_name", "model_name", "quota", "prompt_tokens",
	"completion_tokens", "use_time", "is_stream", "channel",
	"channel_name", "token_id", "group", "ip", "request_id",
	"upstream_request_id", "other",
}

// resolveLogExportFormat validates the ?format= query param and falls back to
// CSV when omitted. Invalid values produce a 400 so the download frontend can
// surface a real error instead of a malformed file.
func resolveLogExportFormat(c *gin.Context) (string, bool) {
	format := c.Query("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid format",
		})
		return "", false
	}
	return format, true
}

// parseLogExportFilter mirrors the query params accepted by the log list
// endpoints. The username dimension is only honored for the admin export;
// user exports are always scoped by the authenticated user id.
func parseLogExportFilter(c *gin.Context, includeUsername bool) model.LogListFilter {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	f := model.LogListFilter{
		LogType:           logType,
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		ModelName:         c.Query("model_name"),
		TokenName:         c.Query("token_name"),
		Channel:           channel,
		Group:             c.Query("group"),
		RequestId:         c.Query("request_id"),
		UpstreamRequestId: c.Query("upstream_request_id"),
	}
	if includeUsername {
		f.Username = c.Query("username")
	}
	return f
}

func ExportAllLogs(c *gin.Context) {
	format, ok := resolveLogExportFormat(c)
	if !ok {
		return
	}
	logs, truncated, err := model.GetAllLogsForExport(parseLogExportFilter(c, true))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	writeLogExport(c, logs, truncated, format)
}

func ExportUserLogs(c *gin.Context) {
	format, ok := resolveLogExportFormat(c)
	if !ok {
		return
	}
	userId := c.GetInt("id")
	logs, truncated, err := model.GetUserLogsForExport(userId, parseLogExportFilter(c, false))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	writeLogExport(c, logs, truncated, format)
}

func writeLogExport(c *gin.Context, logs []*model.Log, truncated bool, format string) {
	filename := fmt.Sprintf("usage_logs_%s.%s", time.Now().Format("20060102_150405"), format)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if truncated {
		c.Header("X-Export-Truncated", "true")
	}

	if format == "json" {
		data, err := common.Marshal(logs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	// UTF-8 BOM so spreadsheet applications detect the encoding on open.
	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write(logExportCSVHeader)
	for i := range logs {
		_ = writer.Write(logExportCSVRow(logs[i]))
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		common.SysError("failed to write usage log export CSV: " + err.Error())
	}
}

func logExportCSVRow(log *model.Log) []string {
	return []string{
		strconv.Itoa(log.Id),
		strconv.Itoa(log.UserId),
		strconv.FormatInt(log.CreatedAt, 10),
		strconv.Itoa(log.Type),
		log.Content,
		log.Username,
		log.TokenName,
		log.ModelName,
		strconv.Itoa(log.Quota),
		strconv.Itoa(log.PromptTokens),
		strconv.Itoa(log.CompletionTokens),
		strconv.Itoa(log.UseTime),
		strconv.FormatBool(log.IsStream),
		strconv.Itoa(log.ChannelId),
		log.ChannelName,
		strconv.Itoa(log.TokenId),
		log.Group,
		log.Ip,
		log.RequestId,
		log.UpstreamRequestId,
		log.Other,
	}
}
