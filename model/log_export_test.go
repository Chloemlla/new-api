package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAllLogsForExportTruncatesAtLimit(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)

	originalLimit := logExportLimit
	logExportLimit = 2
	t.Cleanup(func() { logExportLimit = originalLimit })

	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 1000}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 2000}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 3000}).Error)

	logs, truncated, err := GetAllLogsForExport(LogListFilter{})
	require.NoError(t, err)
	require.True(t, truncated)
	require.Len(t, logs, 2)
	// Newest rows win when the result set is capped.
	require.Equal(t, int64(3000), logs[0].CreatedAt)
}

func TestGetUserLogsForExportStripsAdminAndAuditFields(t *testing.T) {
	truncateTables(t)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)

	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 1000,
		Other: `{"admin_info":{"version":"1.0"},"audit_info":{"route":"/x"},"model_ratio":1.0}`,
	}).Error)

	logs, truncated, err := GetUserLogsForExport(1, LogListFilter{})
	require.NoError(t, err)
	require.False(t, truncated)
	require.Len(t, logs, 1)
	require.NotContains(t, logs[0].Other, "admin_info")
	require.NotContains(t, logs[0].Other, "audit_info")
	require.Contains(t, logs[0].Other, "model_ratio")
}
