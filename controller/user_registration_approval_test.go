package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Register/Login render translated messages through the i18n bundle, which
	// is only loaded by the production main. Initialize it here so tests can
	// exercise those paths (i18n.Init is guarded by sync.Once).
	_ = i18n.Init()
}

func performRegisterRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	Register(c)
	return recorder
}

func performLoginRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	Login(c)
	return recorder
}

// TestRegisterWithApprovalCreatesPendingUser verifies that with approval enabled
// a new registration is stored as pending, receives no starting quota and is
// not issued a default token until approved.
func TestRegisterWithApprovalCreatesPendingUser(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousApproval := common.UserRegistrationApprovalEnabled
	previousQuota := common.QuotaForNewUser
	common.UserRegistrationApprovalEnabled = true
	common.QuotaForNewUser = 10000
	t.Cleanup(func() {
		common.UserRegistrationApprovalEnabled = previousApproval
		common.QuotaForNewUser = previousQuota
	})

	recorder := performRegisterRequest(t, `{"username":"pending-user","password":"password123","email":"pending@example.com"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var user model.User
	require.NoError(t, db.Where("username = ?", "pending-user").First(&user).Error)
	assert.Equal(t, common.UserStatusPending, user.Status)
	assert.Equal(t, 0, user.Quota, "pending user must not receive starting quota before approval")
}

// TestRegisterWithoutApprovalStaysEnabled verifies the feature is opt-in: with
// the approval flag off, registration behaves exactly as before.
func TestRegisterWithoutApprovalStaysEnabled(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousApproval := common.UserRegistrationApprovalEnabled
	previousQuota := common.QuotaForNewUser
	common.UserRegistrationApprovalEnabled = false
	common.QuotaForNewUser = 10000
	t.Cleanup(func() {
		common.UserRegistrationApprovalEnabled = previousApproval
		common.QuotaForNewUser = previousQuota
	})

	recorder := performRegisterRequest(t, `{"username":"normal-user","password":"password123","email":"normal@example.com"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var user model.User
	require.NoError(t, db.Where("username = ?", "normal-user").First(&user).Error)
	assert.Equal(t, common.UserStatusEnabled, user.Status)
	assert.Equal(t, 10000, user.Quota)
}

// TestPendingUserCannotLogin verifies a pending account is refused at login
// with the pending-approval message rather than a generic credential error.
func TestPendingUserCannotLogin(t *testing.T) {
	db := setupManageUserTestDB(t)
	hashed, err := common.Password2Hash("password123")
	require.NoError(t, err)
	user := model.User{
		Username: "pending-login", Password: hashed, Role: common.RoleCommonUser,
		Status: common.UserStatusPending, Group: "default",
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performLoginRequest(t, `{"username":"pending-login","password":"password123"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.NotContains(t, recorder.Body.String(), "user.pending_approval")
	assert.Contains(t, recorder.Body.String(), "pending administrator approval")
}

// TestManageUserApproveEnablesAndGrantsBonuses verifies the admin approve action
// flips a pending account to enabled and pays out the deferred starting quota.
func TestManageUserApproveEnablesAndGrantsBonuses(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 5000
	t.Cleanup(func() { common.QuotaForNewUser = previousQuota })

	user := model.User{
		Username: "approved-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusPending, Group: "default",
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"approve"}`, user.Id))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, updated.Status)
	assert.Equal(t, 5000, updated.Quota, "approval must grant the deferred starting quota")
}

// TestManageUserRejectDisablesPendingUser verifies the reject action disables a
// pending account without granting the starting quota.
func TestManageUserRejectDisablesPendingUser(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 5000
	t.Cleanup(func() { common.QuotaForNewUser = previousQuota })

	user := model.User{
		Username: "rejected-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusPending, Group: "default",
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"reject"}`, user.Id))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, updated.Status)
	assert.Equal(t, 0, updated.Quota, "rejected registration must not be granted the starting quota")
}

// TestManageUserApproveRejectsNonPendingUser verifies approve/reject only apply
// to accounts that are actually pending approval.
func TestManageUserApproveRejectsNonPendingUser(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "already-enabled", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default",
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"approve"}`, user.Id))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "not pending approval")

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, updated.Status, "approve must not change a non-pending account")
}
