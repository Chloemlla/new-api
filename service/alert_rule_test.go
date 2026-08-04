package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalFailureRateRule(t *testing.T) {
	enabled := true
	rule := &model.AlertRule{
		TriggerType:    model.AlertTriggerChannelFailureRate,
		Threshold:      50,
		WindowMinutes:  30,
		MinSampleCount: 5,
		Scope:          model.AlertScopeAll,
		Enabled:        &enabled,
	}
	channels := []*model.Channel{
		{Id: 1, Name: "ok"},
		{Id: 2, Name: "exactly-at-threshold"},
		{Id: 3, Name: "above-threshold"},
		{Id: 4, Name: "low-sample"},
	}
	stats := map[int]channelStats{
		1: {Requests: 8, Errors: 1},  // 11.1%
		2: {Requests: 3, Errors: 3},  // 50.0%, not > 50
		3: {Requests: 2, Errors: 8},  // 80.0%
		4: {Requests: 1, Errors: 1},  // total 2 < min sample 5
	}

	matches := evalFailureRateRule(rule, channels, stats)

	require.Len(t, matches, 1)
	assert.Equal(t, 3, matches[0].Channel.Id)
	assert.InDelta(t, 80.0, matches[0].Metric, 0.001)
}

func TestEvalFailureRateRuleScopes(t *testing.T) {
	enabled := true
	tag := "partner"
	tagRule := &model.AlertRule{
		TriggerType:    model.AlertTriggerChannelFailureRate,
		Threshold:      10,
		WindowMinutes:  30,
		MinSampleCount: 1,
		Scope:          model.AlertScopeTag,
		ChannelTag:     tag,
		Enabled:        &enabled,
	}
	idsRule := &model.AlertRule{
		TriggerType:    model.AlertTriggerChannelFailureRate,
		Threshold:      10,
		WindowMinutes:  30,
		MinSampleCount: 1,
		Scope:          model.AlertScopeIds,
		ChannelIds:     model.ChannelIdList{1},
		Enabled:        &enabled,
	}
	channels := []*model.Channel{
		{Id: 1, Name: "tagged", Tag: &tag},
		{Id: 2, Name: "untagged"},
	}
	stats := map[int]channelStats{
		1: {Requests: 1, Errors: 1},
		2: {Requests: 1, Errors: 1},
	}

	require.Len(t, evalFailureRateRule(tagRule, channels, stats), 1)
	require.Len(t, evalFailureRateRule(idsRule, channels, stats), 1)
}

func TestEvalBalanceRule(t *testing.T) {
	enabled := true
	rule := &model.AlertRule{
		TriggerType: model.AlertTriggerChannelBalance,
		Threshold:   10,
		Scope:       model.AlertScopeAll,
		Enabled:     &enabled,
	}
	channels := []*model.Channel{
		{Id: 1, Name: "low", Balance: 5.5, BalanceUpdatedTime: 100},
		{Id: 2, Name: "high", Balance: 20, BalanceUpdatedTime: 100},
		{Id: 3, Name: "never-probed", Balance: 0, BalanceUpdatedTime: 0},
	}

	matches := evalBalanceRule(rule, channels)

	require.Len(t, matches, 1)
	assert.Equal(t, 1, matches[0].Channel.Id)
	assert.InDelta(t, 5.5, matches[0].Metric, 0.001)
}

func TestAlertRuleCooldownElapsed(t *testing.T) {
	now := int64(1_000_000)
	rule := &model.AlertRule{CooldownMinutes: 60}

	assert.True(t, alertRuleCooldownElapsed(rule, now), "no prior trigger")

	rule.LastTriggeredAt = now - 59*60
	assert.False(t, alertRuleCooldownElapsed(rule, now), "inside cooldown")

	rule.LastTriggeredAt = now - 60*60
	assert.True(t, alertRuleCooldownElapsed(rule, now), "cooldown elapsed")

	rule.CooldownMinutes = 0
	rule.LastTriggeredAt = now
	assert.True(t, alertRuleCooldownElapsed(rule, now), "no cooldown configured")
}

func TestBuildAlertContent(t *testing.T) {
	enabled := true
	rule := &model.AlertRule{
		Name:           "High failure rate",
		TriggerType:    model.AlertTriggerChannelFailureRate,
		Threshold:      50,
		WindowMinutes:  30,
		MinSampleCount: 5,
		Enabled:        &enabled,
	}

	content := buildAlertContent(rule, []alertMatch{
		{Channel: &model.Channel{Id: 3, Name: "worse"}, Metric: 80.0},
	})

	assert.Contains(t, content, "High failure rate")
	assert.Contains(t, content, "#3 worse")
	assert.Contains(t, content, "80.0%")
}
