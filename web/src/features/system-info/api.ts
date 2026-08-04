import { api } from '@/lib/api'

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
export interface SystemInstanceListResponse {
  success: boolean
  message: string
  data?: Array<{
    node_name: string
    status: string
    stale_after_seconds: number
    started_at: number
    last_seen_at: number
  }>
}

export interface SystemInstanceDeleteResponse {
  success: boolean
  message: string
  data?: { deleted_count: number }
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
