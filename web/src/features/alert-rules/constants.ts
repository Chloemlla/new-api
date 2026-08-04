/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { AlertRule } from './types'

export const TRIGGER_TYPE_OPTIONS = [
  { value: 'channel_failure_rate', labelKey: 'Channel failure rate' },
  { value: 'channel_balance', labelKey: 'Channel balance' },
] as const

export const SCOPE_OPTIONS = [
  { value: 'all', labelKey: 'All channels' },
  { value: 'tag', labelKey: 'Channel tag' },
  { value: 'ids', labelKey: 'Specific channels' },
] as const

export const SUCCESS_MESSAGES = {
  ALERT_RULE_CREATED: 'Alert rule created',
  ALERT_RULE_UPDATED: 'Alert rule updated',
  ALERT_RULE_DELETED: 'Alert rule deleted',
  ALERT_RULE_TEST_SENT: 'Test notification sent',
  ALERT_CHECK_COMPLETED: 'Alert check completed',
} as const

export const ERROR_MESSAGES = {
  LOAD_FAILED: 'Failed to load alert rules',
  CREATE_FAILED: 'Failed to create alert rule',
  UPDATE_FAILED: 'Failed to update alert rule',
  DELETE_FAILED: 'Failed to delete alert rule',
  TEST_FAILED: 'Failed to send test notification',
  CHECK_FAILED: 'Failed to run alert check',
} as const

export function triggerTypeLabelKey(
  rule: Pick<AlertRule, 'trigger_type'>
): string {
  return (
    TRIGGER_TYPE_OPTIONS.find((option) => option.value === rule.trigger_type)
      ?.labelKey ?? rule.trigger_type
  )
}

export function scopeLabelKey(rule: Pick<AlertRule, 'scope'>): string {
  return (
    SCOPE_OPTIONS.find((option) => option.value === rule.scope)?.labelKey ??
    rule.scope
  )
}
