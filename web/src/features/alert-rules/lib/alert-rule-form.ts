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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import type { AlertRule, AlertRulePayload } from '../types'

export function getAlertRuleFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().min(1, t('Name is required')),
      enabled: z.boolean(),
      trigger_type: z.enum(['channel_failure_rate', 'channel_balance']),
      threshold: z.number().positive(t('Threshold must be positive')),
      window_minutes: z
        .number()
        .int()
        .min(1, t('Window minutes must be positive')),
      min_sample_count: z.number().int().min(0),
      scope: z.enum(['all', 'tag', 'ids']),
      channel_tag: z.string(),
      channel_ids: z.string(),
      webhook_url: z.string(),
      webhook_secret: z.string(),
      email: z.string(),
      cooldown_minutes: z.number().int().min(0),
    })
    .superRefine((values, ctx) => {
      if (values.scope === 'tag' && values.channel_tag.trim() === '') {
        ctx.addIssue({
          code: 'custom',
          path: ['channel_tag'],
          message: t('Channel tag is required'),
        })
      }
      if (
        values.scope === 'ids' &&
        parseChannelIds(values.channel_ids).length === 0
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['channel_ids'],
          message: t('Enter at least one channel ID'),
        })
      }
      if (values.webhook_url.trim() === '' && values.email.trim() === '') {
        ctx.addIssue({
          code: 'custom',
          path: ['webhook_url'],
          message: t('Add a webhook URL or an email address'),
        })
      }
    })
}

export type AlertRuleFormValues = z.infer<
  ReturnType<typeof getAlertRuleFormSchema>
>

export const ALERT_RULE_FORM_DEFAULT_VALUES: AlertRuleFormValues = {
  name: '',
  enabled: true,
  trigger_type: 'channel_failure_rate',
  threshold: 50,
  window_minutes: 30,
  min_sample_count: 5,
  scope: 'all',
  channel_tag: '',
  channel_ids: '',
  webhook_url: '',
  webhook_secret: '',
  email: '',
  cooldown_minutes: 60,
}

export function parseChannelIds(value: string): number[] {
  return value
    .split(',')
    .map((part) => Number.parseInt(part.trim(), 10))
    .filter((id) => Number.isFinite(id) && id > 0)
}

export function transformRuleToFormValues(
  rule: AlertRule
): AlertRuleFormValues {
  return {
    name: rule.name,
    enabled: rule.enabled ?? true,
    trigger_type: rule.trigger_type,
    threshold: rule.threshold,
    window_minutes: rule.window_minutes,
    min_sample_count: rule.min_sample_count,
    scope: rule.scope,
    channel_tag: rule.channel_tag,
    channel_ids: rule.channel_ids.join(', '),
    webhook_url: rule.webhook_url,
    webhook_secret: rule.webhook_secret,
    email: rule.email,
    cooldown_minutes: rule.cooldown_minutes,
  }
}

export function transformFormValuesToPayload(
  values: AlertRuleFormValues
): AlertRulePayload {
  return {
    name: values.name.trim(),
    enabled: values.enabled,
    trigger_type: values.trigger_type,
    threshold: values.threshold,
    window_minutes: values.window_minutes,
    min_sample_count: values.min_sample_count,
    scope: values.scope,
    channel_tag: values.scope === 'tag' ? values.channel_tag.trim() : '',
    channel_ids:
      values.scope === 'ids' ? parseChannelIds(values.channel_ids) : [],
    webhook_url: values.webhook_url.trim(),
    webhook_secret: values.webhook_secret,
    email: values.email.trim(),
    cooldown_minutes: values.cooldown_minutes,
  }
}
