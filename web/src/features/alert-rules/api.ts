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
import { api } from '@/lib/api'

import type {
  AlertCheckSummary,
  AlertRule,
  AlertRulePayload,
  ApiResponse,
} from './types'

export async function getAlertRules(): Promise<ApiResponse<AlertRule[]>> {
  const res = await api.get('/api/alert-rule/')
  return res.data
}

export async function createAlertRule(
  data: AlertRulePayload
): Promise<ApiResponse<AlertRule>> {
  const res = await api.post('/api/alert-rule/', data)
  return res.data
}

export async function updateAlertRule(
  data: AlertRulePayload
): Promise<ApiResponse<AlertRule>> {
  const res = await api.put('/api/alert-rule/', data)
  return res.data
}

export async function deleteAlertRule(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/alert-rule/${id}`)
  return res.data
}

export async function testAlertRule(payload: {
  webhook_url: string
  webhook_secret: string
  email: string
}): Promise<ApiResponse> {
  const res = await api.post('/api/alert-rule/test', payload)
  return res.data
}

export async function runAlertCheck(): Promise<ApiResponse<AlertCheckSummary>> {
  const res = await api.post('/api/alert-rule/check')
  return res.data
}
