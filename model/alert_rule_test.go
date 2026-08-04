package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelIdListScanAndValue(t *testing.T) {
	// Value marshals the list to a JSON string.
	list := ChannelIdList{1, 3, 5}
	value, err := list.Value()
	require.NoError(t, err)
	assert.Equal(t, "[1,3,5]", value)

	// Scan accepts a JSON byte slice.
	var scanned ChannelIdList
	require.NoError(t, scanned.Scan([]byte("[2,4]")))
	assert.Equal(t, ChannelIdList{2, 4}, scanned)

	// Scan accepts a JSON string.
	var fromString ChannelIdList
	require.NoError(t, fromString.Scan("[]"))
	assert.Empty(t, fromString)

	// Scan of NULL yields an empty list, not an error.
	var nilList ChannelIdList
	require.NoError(t, nilList.Scan(nil))
	assert.Nil(t, nilList)
}

func TestAlertRuleNormalizeDefaults(t *testing.T) {
	rule := &AlertRule{}
	rule.Normalize()

	assert.Equal(t, AlertTriggerChannelFailureRate, rule.TriggerType)
	assert.Equal(t, AlertScopeAll, rule.Scope)
	assert.Equal(t, 30, rule.WindowMinutes)
	assert.Equal(t, 0, rule.MinSampleCount)
	assert.Equal(t, 0, rule.CooldownMinutes)
	require.NotNil(t, rule.Enabled)
	assert.True(t, *rule.Enabled)
}

func TestAlertRuleNormalizeKeepsExplicitValues(t *testing.T) {
	enabled := false
	rule := &AlertRule{
		TriggerType:     AlertTriggerChannelBalance,
		Scope:           AlertScopeIds,
		WindowMinutes:   15,
		MinSampleCount:  0,
		CooldownMinutes: 30,
		Enabled:         &enabled,
	}
	rule.Normalize()

	assert.Equal(t, AlertTriggerChannelBalance, rule.TriggerType)
	assert.Equal(t, AlertScopeIds, rule.Scope)
	assert.Equal(t, 15, rule.WindowMinutes)
	assert.Equal(t, 30, rule.CooldownMinutes)
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
}

func TestAlertRuleBeforeCreateSetsTimestampsAndDefaults(t *testing.T) {
	rule := &AlertRule{Name: "low balance"}
	require.NoError(t, rule.BeforeCreate(nil))

	assert.NotZero(t, rule.CreatedAt)
	assert.NotZero(t, rule.UpdatedAt)
	assert.Equal(t, AlertTriggerChannelFailureRate, rule.TriggerType)
}
