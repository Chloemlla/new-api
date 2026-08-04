package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// channelStats holds the request/error counts observed for one channel inside
// the current alert evaluation window.
type channelStats struct {
	Requests int64
	Errors   int64
}

// alertMatch is a channel that crossed the threshold of one alert rule.
type alertMatch struct {
	Channel *model.Channel
	Metric  float64 // failure rate in percent, or balance in USD
}

// AlertCheckSummary describes one alert rule evaluation pass. It is stored as
// the system task result so the admin can inspect the last run.
type AlertCheckSummary struct {
	RulesChecked  int `json:"rules_checked"`
	AlertsSent    int `json:"alerts_sent"`
	MatchingCount int `json:"matching_count"`
}

var (
	alertRulesCacheMu      sync.RWMutex
	alertRulesCacheEnabled bool
	alertRulesCacheLoaded  bool
)

// refreshAlertRulesCache reloads whether any enabled alert rule exists. The
// cache is kept because the system task scheduler probes Enabled() every poll.
func refreshAlertRulesCache() {
	count, err := model.CountEnabledAlertRules()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to refresh alert rule enabled state: %v", err))
		return
	}
	alertRulesCacheMu.Lock()
	alertRulesCacheEnabled = count > 0
	alertRulesCacheLoaded = true
	alertRulesCacheMu.Unlock()
}

// AlertRulesEnabled reports whether any enabled alert rule exists, so the
// scheduled checker creates no task rows on an idle system.
func AlertRulesEnabled() bool {
	alertRulesCacheMu.RLock()
	loaded := alertRulesCacheLoaded
	enabled := alertRulesCacheEnabled
	alertRulesCacheMu.RUnlock()
	if !loaded {
		refreshAlertRulesCache()
		alertRulesCacheMu.RLock()
		enabled = alertRulesCacheEnabled
		alertRulesCacheMu.RUnlock()
	}
	return enabled
}

// RefreshAlertRulesEnabled forces the cached enabled-state to reload. Call it
// after alert rule CRUD so the scheduled checker starts or stops promptly.
func RefreshAlertRulesEnabled() {
	refreshAlertRulesCache()
}

// AlertCheckInterval is the cadence of the scheduled alert evaluation.
func AlertCheckInterval() time.Duration {
	minutes := common.GetEnvOrDefault("ALERT_CHECK_INTERVAL_MINUTES", 1)
	if minutes < 1 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

// RunAlertRuleCheck evaluates every enabled alert rule once and dispatches
// notifications for the channels that crossed the threshold. It returns a
// summary that is stored as the system task result. ctx cancellation (e.g. from
// a lost task lease) stops the loop between rules.
func RunAlertRuleCheck(ctx context.Context) *AlertCheckSummary {
	summary := &AlertCheckSummary{}
	rules, err := model.GetEnabledAlertRules()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to load alert rules: %v", err))
		return summary
	}
	if len(rules) == 0 {
		return summary
	}

	channels, err := model.GetChannelsForAlert()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to load channels for alert check: %v", err))
		return summary
	}

	now := common.GetTimestamp()
	for _, rule := range rules {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		summary.RulesChecked++
		if !alertRuleCooldownElapsed(rule, now) {
			continue
		}

		var matches []alertMatch
		switch rule.TriggerType {
		case model.AlertTriggerChannelFailureRate:
			if !common.LogConsumeEnabled {
				// Consume logs are the denominator of the failure rate; without
				// them the ratio is meaningless.
				common.SysLog(fmt.Sprintf("alert rule %d skipped: log_consume is disabled", rule.Id))
				continue
			}
			stats, statsErr := channelRequestStats(now - int64(rule.WindowMinutes)*60)
			if statsErr != nil {
				common.SysError(fmt.Sprintf("failed to compute channel request stats for alert rule %d: %v", rule.Id, statsErr))
				continue
			}
			matches = evalFailureRateRule(rule, channels, stats)
		case model.AlertTriggerChannelBalance:
			matches = evalBalanceRule(rule, channels)
		default:
			continue
		}

		if len(matches) == 0 {
			continue
		}
		summary.MatchingCount += len(matches)
		if err := sendAlertNotification(rule, matches); err != nil {
			common.SysError(fmt.Sprintf("alert rule %d notification failed: %v", rule.Id, err))
		} else {
			summary.AlertsSent++
		}
		// Record the attempt (success or failure) so a broken endpoint is not
		// hammered on every run; it retries after the cooldown window.
		if err := model.UpdateAlertRuleLastTriggeredAt(rule.Id, now); err != nil {
			common.SysError(fmt.Sprintf("failed to update alert rule %d last triggered time: %v", rule.Id, err))
		}
	}
	return summary
}

func alertRuleCooldownElapsed(rule *model.AlertRule, now int64) bool {
	if rule.LastTriggeredAt <= 0 || rule.CooldownMinutes <= 0 {
		return true
	}
	return now-rule.LastTriggeredAt >= int64(rule.CooldownMinutes)*60
}

// channelRequestStats counts consume (successful) and error requests per channel
// since the given timestamp from the log database.
func channelRequestStats(since int64) (map[int]channelStats, error) {
	stats := map[int]channelStats{}
	rows := []struct {
		ChannelId int
		Type      int
		Count     int64
	}{}
	err := model.LOG_DB.Model(&model.Log{}).
		Select("channel_id, type, count(*) as count").
		Where("created_at >= ? AND type IN ?", since, []int{model.LogTypeConsume, model.LogTypeError}).
		Group("channel_id, type").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		s := stats[row.ChannelId]
		if row.Type == model.LogTypeError {
			s.Errors += row.Count
		} else {
			s.Requests += row.Count
		}
		stats[row.ChannelId] = s
	}
	return stats, nil
}

func evalFailureRateRule(rule *model.AlertRule, channels []*model.Channel, stats map[int]channelStats) []alertMatch {
	var matches []alertMatch
	for _, channel := range channels {
		if !alertRuleMatchesChannel(rule, channel) {
			continue
		}
		s := stats[channel.Id]
		total := s.Requests + s.Errors
		if total < int64(rule.MinSampleCount) || total == 0 {
			continue
		}
		rate := float64(s.Errors) * 100 / float64(total)
		if rate > rule.Threshold {
			matches = append(matches, alertMatch{Channel: channel, Metric: rate})
		}
	}
	return matches
}

func evalBalanceRule(rule *model.AlertRule, channels []*model.Channel) []alertMatch {
	var matches []alertMatch
	for _, channel := range channels {
		if !alertRuleMatchesChannel(rule, channel) {
			continue
		}
		// Ignore channels whose balance was never reported: an unprobed zero
		// balance would otherwise trigger the rule spuriously.
		if channel.BalanceUpdatedTime <= 0 {
			continue
		}
		if channel.Balance < rule.Threshold {
			matches = append(matches, alertMatch{Channel: channel, Metric: channel.Balance})
		}
	}
	return matches
}

func alertRuleMatchesChannel(rule *model.AlertRule, channel *model.Channel) bool {
	switch rule.Scope {
	case model.AlertScopeTag:
		return channel.GetTag() != "" && channel.GetTag() == rule.ChannelTag
	case model.AlertScopeIds:
		for _, id := range rule.ChannelIds {
			if id == channel.Id {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func sendAlertNotification(rule *model.AlertRule, matches []alertMatch) error {
	subject := "[Alert] " + rule.Name
	content := buildAlertContent(rule, matches)
	notify := dto.NewNotify(dto.NotifyTypeAlert, subject, content, nil)

	var errs []string
	if rule.WebhookUrl != "" {
		if err := SendWebhookNotify(rule.WebhookUrl, rule.WebhookSecret, notify); err != nil {
			errs = append(errs, fmt.Sprintf("webhook: %v", err))
		}
	}
	if rule.Email != "" {
		if err := common.SendEmail(subject, rule.Email, content); err != nil {
			errs = append(errs, fmt.Sprintf("email: %v", err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func buildAlertContent(rule *model.AlertRule, matches []alertMatch) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Rule: %s\n", rule.Name)
	switch rule.TriggerType {
	case model.AlertTriggerChannelFailureRate:
		fmt.Fprintf(&b, "Trigger: channel failure rate > %.0f%%\n", rule.Threshold)
		fmt.Fprintf(&b, "Window: %d minutes (min sample: %d)\n", rule.WindowMinutes, rule.MinSampleCount)
	case model.AlertTriggerChannelBalance:
		fmt.Fprintf(&b, "Trigger: channel balance < $%.2f\n", rule.Threshold)
	}
	fmt.Fprintf(&b, "Channels (%d):\n", len(matches))
	for _, m := range matches {
		switch rule.TriggerType {
		case model.AlertTriggerChannelFailureRate:
			fmt.Fprintf(&b, "  - #%d %s: %.1f%% failure rate\n", m.Channel.Id, m.Channel.Name, m.Metric)
		case model.AlertTriggerChannelBalance:
			fmt.Fprintf(&b, "  - #%d %s: balance $%.2f\n", m.Channel.Id, m.Channel.Name, m.Metric)
		}
	}
	return b.String()
}

// SendAlertTestNotification sends a sample alert to the given webhook URL and/or
// email address so the admin can verify a rule's notification targets work.
func SendAlertTestNotification(webhookURL string, webhookSecret string, email string) error {
	subject := "[Alert] New API Test Notification"
	content := "This is a test alert notification from New API. If you received this, your alert notification configuration is working correctly."
	notify := dto.NewNotify(dto.NotifyTypeAlert, subject, content, nil)

	var errs []string
	if webhookURL != "" {
		if err := SendWebhookNotify(webhookURL, webhookSecret, notify); err != nil {
			errs = append(errs, fmt.Sprintf("webhook: %v", err))
		}
	}
	if email != "" {
		if err := common.SendEmail(subject, email, content); err != nil {
			errs = append(errs, fmt.Sprintf("email: %v", err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
