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
import type { SystemTaskFilters } from '@/features/system-settings/types'
import { api } from '@/lib/api'
import type {
  SystemInstanceDeleteResponse,
  SystemInstanceListResponse,
  SystemTaskHistoryDeleteResponse,
} from './types'

export interface DashboardHealthData {
  system: {
    cpu_usage: number
    memory_usage: number
    disk_usage: number
    monitor_enabled: boolean
  }
  channels: {
    total: number
    enabled: number
    open_breakers: number
    in_flight_total: number
  }
  request_queue: {
    enabled: boolean
    active_requests: number
    queue_size: number
    max_concurrency: number
    avg_wait_ms?: number
  }
  adaptive_limit: {
    enabled: boolean
    current_limit: number
  }
}

export interface ChannelHealthItem {
  channel_id: number
  channel_name: string
  last_checked: string
  success: boolean
  latency_ms: number
  error_msg?: string
  consecutive_failures: number
}

export interface CircuitBreakerItem {
  channel_id: number
  state: 'closed' | 'open' | 'half_open'
}

export async function getDashboardHealth(): Promise<DashboardHealthData> {
  const res = await api.get<{ success: boolean; data: DashboardHealthData }>(
    '/api/performance/dashboard'
  )
  return res.data.data
}

export async function getChannelHealth(): Promise<ChannelHealthItem[]> {
  const res = await api.get<{ success: boolean; data: ChannelHealthItem[] }>(
    '/api/performance/channel-health'
  )
  return res.data.data
}

export async function getCircuitBreakers(): Promise<CircuitBreakerItem[]> {
  const res = await api.get<{ success: boolean; data: CircuitBreakerItem[] }>(
    '/api/performance/circuit-breakers'
  )
  return res.data.data
}

export async function getInFlight(): Promise<Record<number, number>> {
  const res = await api.get<{ success: boolean; data: Record<number, number> }>(
    '/api/performance/in-flight'
  )
  return res.data.data
}

export async function reloadConfig(): Promise<void> {
  await api.post('/api/option/reload')
}

export async function listSystemInstances(): Promise<SystemInstanceListResponse> {
  const res = await api.get<SystemInstanceListResponse>(
    "/api/performance/system-instances"
  )
  return res.data
}

export async function deleteStaleSystemInstance(nodeName: string): Promise<SystemInstanceDeleteResponse> {
  const res = await api.post<SystemInstanceDeleteResponse>(
    "/api/performance/system-instances/delete",
    { node_name: nodeName }
  )
  return res.data
}

export async function deleteStaleSystemInstances(): Promise<SystemInstanceDeleteResponse> {
  const res = await api.post<SystemInstanceDeleteResponse>(
    "/api/performance/system-instances/delete-stale"
  )
  return res.data
}

export async function deleteSystemTaskHistory(
  filters: Pick<SystemTaskFilters, 'type' | 'status'>
) {
  const res = await api.delete<SystemTaskHistoryDeleteResponse>(
    '/api/system-task/history',
    { params: filters }
  )
  return res.data
}
