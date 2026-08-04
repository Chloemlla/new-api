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
import type { TimeGranularity } from '@/lib/time'

/**
 * Per-channel quota aggregation row served by `/api/data/channels`.
 * One row represents the aggregated usage of a single channel within a
 * single time bucket (`created_at`, hour-aligned in `quota_data`).
 */
export interface ChannelQuotaDataItem {
  channel_id?: number
  channel_name?: string
  created_at: number
  token_used?: number
  count?: number
  quota?: number
}

export interface ChannelAnalyticsFilters {
  start_timestamp?: Date
  end_timestamp?: Date
  time_granularity: TimeGranularity
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type VChartSpec = Record<string, any>

export interface ProcessedChannelChartData {
  spec_cost_trend: VChartSpec
  spec_request_trend: VChartSpec
  spec_cost_rank: VChartSpec
  spec_request_rank: VChartSpec
  totalCostDisplay: string
  totalCountDisplay: string
  totalTokenDisplay: string
}
