package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetAllAlertRules(c *gin.Context) {
	rules, err := model.GetAllAlertRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rules,
	})
	return
}

func GetAlertRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rule, err := model.GetAlertRuleById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rule,
	})
	return
}

func AddAlertRule(c *gin.Context) {
	rule := model.AlertRule{}
	if err := c.ShouldBindJSON(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	rule.Normalize()
	if err := validateAlertRule(&rule); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := rule.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	service.RefreshAlertRulesEnabled()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rule,
	})
	return
}

func UpdateAlertRule(c *gin.Context) {
	rule := model.AlertRule{}
	if err := c.ShouldBindJSON(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	existing, err := model.GetAlertRuleById(rule.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	existing.Name = rule.Name
	existing.Enabled = rule.Enabled
	existing.TriggerType = rule.TriggerType
	existing.Threshold = rule.Threshold
	existing.WindowMinutes = rule.WindowMinutes
	existing.MinSampleCount = rule.MinSampleCount
	existing.Scope = rule.Scope
	existing.ChannelTag = rule.ChannelTag
	existing.ChannelIds = rule.ChannelIds
	existing.WebhookUrl = rule.WebhookUrl
	existing.WebhookSecret = rule.WebhookSecret
	existing.Email = rule.Email
	existing.CooldownMinutes = rule.CooldownMinutes
	existing.Normalize()
	if err := validateAlertRule(existing); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := existing.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	service.RefreshAlertRulesEnabled()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    existing,
	})
	return
}

func DeleteAlertRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rule, err := model.GetAlertRuleById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := rule.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	service.RefreshAlertRulesEnabled()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// TestAlertRule sends a sample alert to the given webhook/email so the admin can
// verify a rule's notification targets before relying on them.
func TestAlertRule(c *gin.Context) {
	var req struct {
		WebhookUrl    string `json:"webhook_url"`
		WebhookSecret string `json:"webhook_secret"`
		Email         string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.WebhookUrl == "" && req.Email == "" {
		common.ApiErrorMsg(c, "at least one notification target (webhook or email) is required")
		return
	}
	if err := service.SendAlertTestNotification(req.WebhookUrl, req.WebhookSecret, req.Email); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// RunAlertRuleCheck forces an immediate alert rule evaluation and returns the
// summary, so the admin can verify rules without waiting for the scheduled run.
func RunAlertRuleCheck(c *gin.Context) {
	summary := service.RunAlertRuleCheck(nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    summary,
	})
	return
}

func validateAlertRule(rule *model.AlertRule) error {
	if rule.Name == "" {
		return errors.New("alert rule name is required")
	}
	switch rule.TriggerType {
	case model.AlertTriggerChannelFailureRate, model.AlertTriggerChannelBalance:
	default:
		return errors.New("invalid trigger type")
	}
	if rule.Threshold <= 0 {
		return errors.New("threshold must be positive")
	}
	if rule.TriggerType == model.AlertTriggerChannelFailureRate {
		if rule.WindowMinutes < 1 {
			return errors.New("window minutes must be positive")
		}
		if rule.MinSampleCount < 0 {
			return errors.New("min sample count cannot be negative")
		}
	}
	switch rule.Scope {
	case model.AlertScopeAll, model.AlertScopeTag, model.AlertScopeIds:
	default:
		return errors.New("invalid channel scope")
	}
	if rule.Scope == model.AlertScopeTag && rule.ChannelTag == "" {
		return errors.New("channel tag is required for tag scope")
	}
	if rule.Scope == model.AlertScopeIds && len(rule.ChannelIds) == 0 {
		return errors.New("at least one channel is required for ids scope")
	}
	if rule.CooldownMinutes < 0 {
		return errors.New("cooldown minutes cannot be negative")
	}
	if rule.WebhookUrl == "" && rule.Email == "" {
		return errors.New("at least one notification target (webhook or email) is required")
	}
	return nil
}
