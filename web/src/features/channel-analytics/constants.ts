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
export const CHANNEL_ANALYTICS_FILTERS_STORAGE_KEY = 'channel_analytics_filters'

export const DEFAULT_CHANNEL_ANALYTICS_GRANULARITY = 'day' as const

// Rolling range matched to the selected granularity, mirroring the dashboard.
export const TIME_RANGE_BY_GRANULARITY = {
  hour: 1,
  day: 7,
  week: 30,
} as const

export const TIME_RANGE_PRESETS = [
  { label: '1 Day', days: 1 },
  { label: '7 Days', days: 7 },
  { label: '14 Days', days: 14 },
  { label: '29 Days', days: 29 },
] as const

export const TIME_GRANULARITY_OPTIONS = [
  { label: 'Hour', value: 'hour' },
  { label: 'Day', value: 'day' },
  { label: 'Week', value: 'week' },
] as const

export const CHANNEL_ANALYTICS_TOP_LIMIT_OPTIONS = [10, 20, 50] as const

export const DEFAULT_CHANNEL_ANALYTICS_TOP_LIMIT = 10

export const CHANNEL_ANALYTICS_PALETTE = [
  '#5B8FF9',
  '#5AD8A6',
  '#F6BD16',
  '#E8684A',
  '#6DC8EC',
  '#9270CA',
  '#FF9D4D',
  '#269A99',
  '#FF99C3',
  '#5D7092',
  '#A5FCF5',
  '#C0EBFF',
]
