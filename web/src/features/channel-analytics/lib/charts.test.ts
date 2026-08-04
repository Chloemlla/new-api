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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { ChannelQuotaDataItem } from '../types'
import { processChannelChartData } from './charts'

const rows: ChannelQuotaDataItem[] = [
  // Channel "east": 150 quota / 3 requests across two hours.
  {
    channel_id: 1,
    channel_name: 'east',
    created_at: 1000,
    quota: 100,
    count: 2,
    token_used: 40,
  },
  {
    channel_id: 1,
    channel_name: 'east',
    created_at: 1100,
    quota: 50,
    count: 1,
    token_used: 20,
  },
  // Channel "west": 25 quota / 3 requests.
  {
    channel_id: 2,
    channel_name: 'west',
    created_at: 1000,
    quota: 25,
    count: 3,
    token_used: 10,
  },
  // Channel "north": low cost but the highest request volume.
  {
    channel_id: 3,
    channel_name: 'north',
    created_at: 1000,
    quota: 10,
    count: 100,
    token_used: 5,
  },
]

function costRows(result: ReturnType<typeof processChannelChartData>) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const values = result.spec_cost_trend.data[0].values as any[]
  return values.map((v) => ({ Channel: v.Channel, rawQuota: v.rawQuota }))
}

function requestRows(result: ReturnType<typeof processChannelChartData>) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const values = result.spec_request_trend.data[0].values as any[]
  return values.map((v) => ({ Channel: v.Channel, Count: v.Count }))
}

describe('processChannelChartData', () => {
  test('aggregates per-channel usage into time buckets and rankings', () => {
    const result = processChannelChartData(rows, 'day', 10, (key) => key)

    // Cost trend: one time bucket with every channel as its own series.
    assert.deepEqual(costRows(result), [
      { Channel: 'east', rawQuota: 150 },
      { Channel: 'west', rawQuota: 25 },
      { Channel: 'north', rawQuota: 10 },
    ])
    // Request trend keeps the same bucket but the request counts, sorted by
    // volume descending so the busiest channel reads first.
    assert.deepEqual(requestRows(result), [
      { Channel: 'north', Count: 100 },
      { Channel: 'east', Count: 3 },
      { Channel: 'west', Count: 3 },
    ])
    // Cost ranking is sorted by total cost, descending.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const costRank = result.spec_cost_rank.data[0].values as any[]
    assert.deepEqual(
      costRank.map((v) => v.Channel),
      ['east', 'west', 'north']
    )
    // Request ranking is sorted by total requests, descending.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const requestRank = result.spec_request_rank.data[0].values as any[]
    assert.equal(requestRank[0].Channel, 'north')
    assert.equal(requestRank[0].Count, 100)
    // Totals combine every channel.
    assert.equal(result.totalCountDisplay, '106')
  })

  test('buckets channels below the display limit into Other channels', () => {
    const result = processChannelChartData(rows, 'day', 2, (key) => key)

    assert.deepEqual(costRows(result), [
      { Channel: 'east', rawQuota: 150 },
      { Channel: 'west', rawQuota: 25 },
      { Channel: 'Other channels', rawQuota: 10 },
    ])
    // Request trend ranks by requests, so "north" is in the top two and
    // "west" falls into the Other bucket instead.
    assert.deepEqual(requestRows(result), [
      { Channel: 'north', Count: 100 },
      { Channel: 'east', Count: 3 },
      { Channel: 'Other channels', Count: 3 },
    ])
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const costRank = result.spec_cost_rank.data[0].values as any[]
    assert.deepEqual(
      costRank.map((v) => v.Channel),
      ['east', 'west', 'Other channels']
    )
    assert.equal(costRank[2].rawQuota, 10)
  })

  test('returns empty specs for no data', () => {
    const result = processChannelChartData([], 'day', 10, (key) => key)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    assert.deepEqual(result.spec_cost_trend.data[0].values as any[], [])
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    assert.deepEqual(result.spec_request_rank.data[0].values as any[], [])
    assert.equal(result.totalCountDisplay, '0')
    assert.equal(result.totalTokenDisplay, '0')
  })

  test('falls back to a generated label when a channel has no name', () => {
    const unnamedRows: ChannelQuotaDataItem[] = [
      { channel_id: 7, created_at: 1000, quota: 5, count: 1 },
      { created_at: 1000, quota: 3, count: 2 },
    ]
    const result = processChannelChartData(unnamedRows, 'day', 10, (key) => key)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const costRank = result.spec_cost_rank.data[0].values as any[]
    assert.deepEqual(
      costRank.map((v) => v.Channel),
      ['channel-7', 'Unknown']
    )
  })
})
