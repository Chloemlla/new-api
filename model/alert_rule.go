package model

import (
	"database/sql/driver"
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Alert rule trigger types.
const (
	AlertTriggerChannelFailureRate = "channel_failure_rate"
	AlertTriggerChannelBalance     = "channel_balance"
)

// Alert rule channel scopes.
const (
	AlertScopeAll = "all"
	AlertScopeTag = "tag"
	AlertScopeIds = "ids"
)

// ChannelIdList stores a list of channel ids as a JSON text column while
// marshaling to a native array in the API payload.
type ChannelIdList []int

// Value implements driver.Valuer.
func (l ChannelIdList) Value() (driver.Value, error) {
	data, err := common.Marshal(l)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// Scan implements sql.Scanner.
func (l *ChannelIdList) Scan(value interface{}) error {
	if value == nil {
		*l = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.New("unsupported type for channel id list")
	}
	if len(data) == 0 {
		*l = nil
		return nil
	}
	return common.Unmarshal(data, l)
}

// AlertRule describes one configurable alert: which channels to watch, the
// trigger (failure rate or balance) and where the notification is delivered.
type AlertRule struct {
	Id              int           `json:"id" gorm:"primaryKey"`
	Name            string        `json:"name" gorm:"type:varchar(128);index"`
	Enabled         *bool         `json:"enabled"`
	TriggerType     string        `json:"trigger_type" gorm:"type:varchar(32);index"`
	Threshold       float64       `json:"threshold"`
	WindowMinutes   int           `json:"window_minutes"`
	MinSampleCount  int           `json:"min_sample_count"`
	Scope           string        `json:"scope" gorm:"type:varchar(16)"`
	ChannelTag      string        `json:"channel_tag" gorm:"type:varchar(128)"`
	ChannelIds      ChannelIdList `json:"channel_ids" gorm:"type:text"`
	WebhookUrl      string        `json:"webhook_url" gorm:"type:varchar(2048)"`
	WebhookSecret   string        `json:"webhook_secret" gorm:"type:varchar(256)"`
	Email           string        `json:"email" gorm:"type:varchar(256)"`
	CooldownMinutes int           `json:"cooldown_minutes"`
	LastTriggeredAt int64         `json:"last_triggered_at" gorm:"bigint"`
	CreatedAt       int64         `json:"created_at" gorm:"bigint"`
	UpdatedAt       int64         `json:"updated_at" gorm:"bigint"`
}

// Normalize fills in sensible defaults so a rule created with partial input is
// always valid. Business defaults are enforced here instead of GORM default
// tags for cross-database consistency.
func (r *AlertRule) Normalize() {
	if r.TriggerType == "" {
		r.TriggerType = AlertTriggerChannelFailureRate
	}
	if r.Scope == "" {
		r.Scope = AlertScopeAll
	}
	if r.WindowMinutes <= 0 {
		r.WindowMinutes = 30
	}
	if r.MinSampleCount < 0 {
		r.MinSampleCount = 0
	}
	if r.CooldownMinutes < 0 {
		r.CooldownMinutes = 0
	}
	if r.Enabled == nil {
		r.Enabled = common.GetPointer(true)
	}
}

func (r *AlertRule) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.Normalize()
	return nil
}

func (r *AlertRule) BeforeUpdate(_ *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

func (r *AlertRule) Insert() error {
	return DB.Create(r).Error
}

// Update persists the whole rule. Editing a rule resets LastTriggeredAt so the
// new configuration is evaluated immediately on the next check.
func (r *AlertRule) Update() error {
	r.LastTriggeredAt = 0
	return DB.Save(r).Error
}

func (r *AlertRule) Delete() error {
	return DB.Delete(r).Error
}

func GetAlertRuleById(id int) (*AlertRule, error) {
	if id <= 0 {
		return nil, errors.New("invalid alert rule id")
	}
	rule := &AlertRule{}
	err := DB.Where("id = ?", id).First(rule).Error
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func GetAllAlertRules() ([]*AlertRule, error) {
	var rules []*AlertRule
	err := DB.Order("id asc").Find(&rules).Error
	return rules, err
}

func GetEnabledAlertRules() ([]*AlertRule, error) {
	var rules []*AlertRule
	err := DB.Where("enabled = ?", true).Find(&rules).Error
	return rules, err
}

func CountEnabledAlertRules() (int64, error) {
	var count int64
	err := DB.Model(&AlertRule{}).Where("enabled = ?", true).Count(&count).Error
	return count, err
}

func UpdateAlertRuleLastTriggeredAt(id int, timestamp int64) error {
	return DB.Model(&AlertRule{}).Where("id = ?", id).Updates(map[string]any{
		"last_triggered_at": timestamp,
		"updated_at":        common.GetTimestamp(),
	}).Error
}

// GetChannelsForAlert returns only the channel columns the alert checker needs.
// Keys are omitted so checking never loads secrets into memory.
func GetChannelsForAlert() ([]*Channel, error) {
	var channels []*Channel
	err := DB.Select("id", "name", "status", "balance", "balance_updated_time", "tag").Find(&channels).Error
	return channels, err
}
