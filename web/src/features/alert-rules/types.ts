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
import { z } from 'zod'

export const alertRuleSchema = z.object({
  id: z.number(),
  name: z.string(),
  enabled: z.boolean().nullable(),
  trigger_type: z.enum(['channel_failure_rate', 'channel_balance']),
  threshold: z.number(),
  window_minutes: z.number(),
  min_sample_count: z.number(),
  scope: z.enum(['all', 'tag', 'ids']),
  channel_tag: z.string(),
  channel_ids: z.array(z.number()),
  webhook_url: z.string(),
  webhook_secret: z.string(),
  email: z.string(),
  cooldown_minutes: z.number(),
  last_triggered_at: z.number(),
  created_at: z.number(),
  updated_at: z.number(),
})

export type AlertRule = z.infer<typeof alertRuleSchema>

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface AlertRulePayload {
  id?: number
  name: string
  enabled: boolean
  trigger_type: 'channel_failure_rate' | 'channel_balance'
  threshold: number
  window_minutes: number
  min_sample_count: number
  scope: 'all' | 'tag' | 'ids'
  channel_tag: string
  channel_ids: number[]
  webhook_url: string
  webhook_secret: string
  email: string
  cooldown_minutes: number
}

export interface AlertCheckSummary {
  rules_checked: number
  alerts_sent: number
  matching_count: number
}

export type AlertRuleDialogType = 'create' | 'update' | 'delete' | 'test'
