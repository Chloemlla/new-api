package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenIPWhitelistTest(t *testing.T, username string, affCode string) *model.User {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitColumnNames()
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
	})
	user := &model.User{
		Username: username, Password: "password-placeholder", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: affCode,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func createTokenForIPTest(t *testing.T, userID int, key string, allowIps *string) *model.Token {
	t.Helper()
	token := &model.Token{
		UserId: userID, Key: key, Name: "ip-whitelist-test", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true, AllowIps: allowIps,
	}
	require.NoError(t, model.DB.Create(token).Error)
	return token
}

func allowIpsPtr(s string) *string {
	return &s
}

func newTokenAuthIPRequest(path string, authHeader string, remoteAddr string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", authHeader)
	request.RemoteAddr = remoteAddr
	return request
}

func TestTokenAuthEnforcesIPWhitelist(t *testing.T) {
	user := setupTokenIPWhitelistTest(t, "ip-token-user", "ip-token-aff")
	createTokenForIPTest(t, user.Id, "iptestallowed", allowIpsPtr("203.0.113.5\n198.51.100.0/24\n2001:db8::/32"))
	createTokenForIPTest(t, user.Id, "iptestopen", nil)

	router := gin.New()
	router.GET("/v1/chat/completions", TokenAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name        string
		authHeader  string
		remoteAddr  string
		wantCode    int
		wantMessage string
	}{
		{name: "exact whitelisted IPv4", authHeader: "Bearer sk-iptestallowed", remoteAddr: "203.0.113.5:1234", wantCode: http.StatusNoContent},
		{name: "whitelisted IPv4 CIDR", authHeader: "Bearer sk-iptestallowed", remoteAddr: "198.51.100.99:1234", wantCode: http.StatusNoContent},
		{name: "whitelisted IPv6 CIDR", authHeader: "Bearer sk-iptestallowed", remoteAddr: "[2001:db8:1234::1]:1234", wantCode: http.StatusNoContent},
		{name: "IP outside whitelist rejected", authHeader: "Bearer sk-iptestallowed", remoteAddr: "192.0.2.10:1234", wantCode: http.StatusForbidden, wantMessage: "access_denied"},
		{name: "unparseable client IP rejected", authHeader: "Bearer sk-iptestallowed", remoteAddr: "not-an-ip:1234", wantCode: http.StatusForbidden},
		{name: "token without whitelist allows any IP", authHeader: "Bearer sk-iptestopen", remoteAddr: "192.0.2.10:1234", wantCode: http.StatusNoContent},
		{name: "token without whitelist allows loopback", authHeader: "Bearer sk-iptestopen", remoteAddr: "127.0.0.1:1234", wantCode: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newTokenAuthIPRequest("/v1/chat/completions", tt.authHeader, tt.remoteAddr))
			assert.Equal(t, tt.wantCode, response.Code)
			if tt.wantMessage != "" {
				assert.Contains(t, response.Body.String(), tt.wantMessage)
			}
		})
	}
}

func TestTokenAuthReadOnlyEnforcesIPWhitelist(t *testing.T) {
	user := setupTokenIPWhitelistTest(t, "readonly-token-user", "readonly-token-aff")
	createTokenForIPTest(t, user.Id, "readonlywhitelisted", allowIpsPtr("203.0.113.5"))
	createTokenForIPTest(t, user.Id, "readonlyopen", nil)

	router := gin.New()
	router.GET("/api/usage/token", TokenAuthReadOnly(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name        string
		authHeader  string
		remoteAddr  string
		wantCode    int
		wantMessage string
	}{
		{name: "whitelisted IP", authHeader: "Bearer sk-readonlywhitelisted", remoteAddr: "203.0.113.5:1234", wantCode: http.StatusNoContent},
		{name: "IP outside whitelist rejected", authHeader: "Bearer sk-readonlywhitelisted", remoteAddr: "192.0.2.10:1234", wantCode: http.StatusForbidden, wantMessage: "success\":false"},
		{name: "unparseable client IP rejected", authHeader: "Bearer sk-readonlywhitelisted", remoteAddr: "not-an-ip:1234", wantCode: http.StatusForbidden, wantMessage: "success\":false"},
		{name: "token without whitelist allows any IP", authHeader: "Bearer sk-readonlyopen", remoteAddr: "192.0.2.10:1234", wantCode: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newTokenAuthIPRequest("/api/usage/token", tt.authHeader, tt.remoteAddr))
			assert.Equal(t, tt.wantCode, response.Code)
			if tt.wantMessage != "" {
				assert.Contains(t, response.Body.String(), tt.wantMessage)
			}
		})
	}
}
