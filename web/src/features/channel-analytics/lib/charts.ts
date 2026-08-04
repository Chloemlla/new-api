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
import { getCurrencyDisplay } from '@/lib/currency'
import { formatQuota } from '@/lib/format'
import { formatChartTime, type TimeGranularity } from '@/lib/time'

import {
  CHANNEL_ANALYTICS_PALETTE,
  DEFAULT_CHANNEL_ANALYTICS_TOP_LIMIT,
} from '../constants'
import type { ChannelQuotaDataItem, ProcessedChannelChartData } from '../types'

type TFunction = (key: string) => string

type TooltipLineItem = {
  key: string
  value: string | number
  datum?: Record<string, unknown>
  hasShape?: boolean
  shapeType?: string
  shapeFill?: string
  shapeStroke?: string
  shapeSize?: number
}

interface ChannelStats {
  quota: number
  count: number
  tokens: number
}

interface TrendDatum {
  Time: string
  Channel: string
  rawQuota: number
  Usage: number
  Count: number
  TimeSum: number
}

interface RankDatum {
  Channel: string
  rawQuota: number
  Count: number
  Usage: number
}

function channelLabel(
  item: ChannelQuotaDataItem,
  unknownLabel: string
): string {
  if (item.channel_name) return item.channel_name
  if (item.channel_id) return `channel-${item.channel_id}`
  return unknownLabel
}

export function processChannelChartData(
  data: ChannelQuotaDataItem[],
  timeGranularity: TimeGranularity = 'day',
  topLimit = DEFAULT_CHANNEL_ANALYTICS_TOP_LIMIT,
  t?: TFunction
): ProcessedChannelChartData {
  const tt: TFunction = t ?? ((x) => x)
  const otherLabel = tt('Other channels')
  const unknownLabel = tt('Unknown')
  const { config } = getCurrencyDisplay()
  const quotaPerUnit = config.quotaPerUnit

  const formatInt = (value: number) =>
    Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value)
  const formatCost = (rawQuota: number) => formatQuota(rawQuota)

  const emptyResult: ProcessedChannelChartData = {
    spec_cost_trend: {
      type: 'bar',
      data: [{ id: 'costTrend', values: [] }],
      xField: 'Time',
      yField: 'Usage',
      seriesField: 'Channel',
      stack: true,
      legends: { visible: true, selectMode: 'single' },
      title: {
        visible: true,
        text: tt('Channel Cost Trend'),
        subtext: tt('No data available'),
      },
      background: { fill: 'transparent' },
    },
    spec_request_trend: {
      type: 'area',
      data: [{ id: 'requestTrend', values: [] }],
      xField: 'Time',
      yField: 'Count',
      seriesField: 'Channel',
      stack: true,
      legends: { visible: true, selectMode: 'single' },
      title: {
        visible: true,
        text: tt('Channel Request Trend'),
        subtext: tt('No data available'),
      },
      background: { fill: 'transparent' },
    },
    spec_cost_rank: {
      type: 'bar',
      data: [{ id: 'costRank', values: [] }],
      xField: 'rawQuota',
      yField: 'Channel',
      seriesField: 'Channel',
      direction: 'horizontal',
      legends: { visible: false },
      title: {
        visible: true,
        text: tt('Channel Cost Ranking'),
        subtext: tt('No data available'),
      },
      background: { fill: 'transparent' },
    },
    spec_request_rank: {
      type: 'bar',
      data: [{ id: 'requestRank', values: [] }],
      xField: 'Count',
      yField: 'Channel',
      seriesField: 'Channel',
      direction: 'horizontal',
      legends: { visible: false },
      title: {
        visible: true,
        text: tt('Channel Request Ranking'),
        subtext: tt('No data available'),
      },
      background: { fill: 'transparent' },
    },
    totalCostDisplay: formatCost(0),
    totalCountDisplay: formatInt(0),
    totalTokenDisplay: formatInt(0),
  }

  if (!data || data.length === 0) {
    return emptyResult
  }

  // Aggregate all metrics by time bucket and channel label.
  const timeChannelMap = new Map<string, Map<string, ChannelStats>>()
  const channelTotalsMap = new Map<string, ChannelStats>()
  data.forEach((item) => {
    const timeKey = formatChartTime(Number(item.created_at), timeGranularity)
    const label = channelLabel(item, unknownLabel)
    const quota = Number(item.quota) || 0
    const count = Number(item.count) || 0
    const tokens = Number(item.token_used) || 0

    let channelMap = timeChannelMap.get(timeKey)
    if (!channelMap) {
      channelMap = new Map()
      timeChannelMap.set(timeKey, channelMap)
    }
    const existing = channelMap.get(label) || { quota: 0, count: 0, tokens: 0 }
    channelMap.set(label, {
      quota: existing.quota + quota,
      count: existing.count + count,
      tokens: existing.tokens + tokens,
    })

    const totalExisting = channelTotalsMap.get(label) || {
      quota: 0,
      count: 0,
      tokens: 0,
    }
    channelTotalsMap.set(label, {
      quota: totalExisting.quota + quota,
      count: totalExisting.count + count,
      tokens: totalExisting.tokens + tokens,
    })
  })

  const sortedTimes = [...timeChannelMap.keys()].sort()
  const costRanked = [...channelTotalsMap.entries()].sort(
    (a, b) => b[1].quota - a[1].quota
  )
  const countRanked = [...channelTotalsMap.entries()].sort(
    (a, b) => b[1].count - a[1].count
  )

  const totalQuota = costRanked.reduce((sum, [, stats]) => sum + stats.quota, 0)
  const totalCount = costRanked.reduce((sum, [, stats]) => sum + stats.count, 0)
  const totalTokens = costRanked.reduce(
    (sum, [, stats]) => sum + stats.tokens,
    0
  )

  // Each chart buckets channels below the display limit into "Other channels".
  const topCostChannels = costRanked.slice(0, topLimit).map(([c]) => c)
  const topCountChannels = countRanked.slice(0, topLimit).map(([c]) => c)
  const hasCostOverflow = costRanked.length > topLimit
  const hasCountOverflow = countRanked.length > topLimit

  // Shared color mapping: every chart gives the same channel the same color.
  const colorDomain = [
    ...new Set([...topCostChannels, ...topCountChannels, otherLabel]),
  ]
  const colorRange = colorDomain.map(
    (_, index) =>
      CHANNEL_ANALYTICS_PALETTE[index % CHANNEL_ANALYTICS_PALETTE.length]
  )
  const channelColor = colorDomain.reduce<Record<string, string>>(
    (acc, label, index) => {
      acc[label] = colorRange[index]
      return acc
    },
    {}
  )
  const color = { type: 'ordinal', domain: colorDomain, range: colorRange }

  const makeCostTooltip = (collapseOverflow: boolean) => ({
    mark: {
      content: [
        {
          key: (datum: Record<string, unknown>) => datum?.Channel,
          value: (datum: Record<string, unknown>) =>
            formatCost(Number(datum?.rawQuota) || 0),
        },
      ],
    },
    dimension: {
      content: [
        {
          key: (datum: Record<string, unknown>) => datum?.Channel,
          value: (datum: Record<string, unknown>) =>
            Number(datum?.rawQuota) || 0,
        },
      ],
      updateContent: (array: TooltipLineItem[]) => {
        if (!collapseOverflow) {
          array.sort((a, b) => (Number(b.value) || 0) - (Number(a.value) || 0))
          for (let i = 0; i < array.length; i++) {
            array[i].value = formatCost(Number(array[i].value) || 0)
          }
          let sum = 0
          for (let i = 0; i < array.length; i++) {
            sum += Number(array[i].datum?.rawQuota) || 0
          }
          array.unshift({ key: tt('Total:'), value: formatCost(sum) })
          return array
        }
        const modelItems = array.filter((item) => item.key !== otherLabel)
        const otherItems = array.filter((item) => item.key === otherLabel)
        modelItems.sort(
          (a, b) =>
            (Number(b.datum?.rawQuota) || 0) - (Number(a.datum?.rawQuota) || 0)
        )
        let totalRaw = 0
        for (const item of [...modelItems, ...otherItems]) {
          totalRaw += Number(item.datum?.rawQuota) || 0
        }
        const formatted = modelItems.map((item) => ({
          ...item,
          value: formatCost(Number(item.datum?.rawQuota) || 0),
        }))
        if (otherItems.length > 0) {
          const otherRaw = otherItems.reduce(
            (sum, item) => sum + (Number(item.datum?.rawQuota) || 0),
            0
          )
          formatted.push({
            key: otherLabel,
            value: formatCost(otherRaw),
            hasShape: true,
            shapeType: 'square',
            shapeFill: channelColor[otherLabel],
            shapeStroke: channelColor[otherLabel],
            shapeSize: 8,
          })
        }
        formatted.unshift({ key: tt('Total:'), value: formatCost(totalRaw) })
        return formatted
      },
    },
  })

  const makeCountTooltip = () => ({
    mark: {
      content: [
        {
          key: (datum: Record<string, unknown>) => datum?.Channel,
          value: (datum: Record<string, unknown>) =>
            formatInt(Number(datum?.Count) || 0),
        },
      ],
    },
    dimension: {
      content: [
        {
          key: (datum: Record<string, unknown>) => datum?.Channel,
          value: (datum: Record<string, unknown>) => Number(datum?.Count) || 0,
        },
      ],
      updateContent: (array: TooltipLineItem[]) => {
        array.sort((a, b) => (Number(b.value) || 0) - (Number(a.value) || 0))
        let sum = 0
        for (let i = 0; i < array.length; i++) {
          const v = Number(array[i].value) || 0
          sum += v
          array[i].value = formatInt(v)
        }
        array.unshift({ key: tt('Total:'), value: formatInt(sum) })
        return array
      },
    },
  })

  // Stacked cost trend: top cost channels over time (Others merged).
  const costTrendValues: TrendDatum[] = []
  sortedTimes.forEach((time) => {
    const channelMap = timeChannelMap.get(time)
    if (!channelMap) return
    let timeSum = 0
    const buckets = new Map<string, ChannelStats>()
    channelMap.forEach((stats, label) => {
      timeSum += stats.quota
      const key = topCostChannels.includes(label) ? label : otherLabel
      const prev = buckets.get(key) || { quota: 0, count: 0, tokens: 0 }
      buckets.set(key, {
        quota: prev.quota + stats.quota,
        count: prev.count + stats.count,
        tokens: prev.tokens + stats.tokens,
      })
    })
    const bucketRows = [...buckets.entries()].map(
      ([label, stats]): TrendDatum => ({
        Time: time,
        Channel: label,
        rawQuota: stats.quota,
        Usage: stats.quota
          ? Number((stats.quota / quotaPerUnit).toFixed(4))
          : 0,
        Count: stats.count,
        TimeSum: timeSum,
      })
    )
    bucketRows.sort((a, b) => b.rawQuota - a.rawQuota)
    costTrendValues.push(...bucketRows)
  })
  costTrendValues.sort((a, b) => a.Time.localeCompare(b.Time))

  // Stacked request trend: top request channels over time (Others merged).
  const requestTrendValues: TrendDatum[] = []
  sortedTimes.forEach((time) => {
    const channelMap = timeChannelMap.get(time)
    if (!channelMap) return
    const buckets = new Map<string, ChannelStats>()
    channelMap.forEach((stats, label) => {
      const key = topCountChannels.includes(label) ? label : otherLabel
      const prev = buckets.get(key) || { quota: 0, count: 0, tokens: 0 }
      buckets.set(key, {
        quota: prev.quota + stats.quota,
        count: prev.count + stats.count,
        tokens: prev.tokens + stats.tokens,
      })
    })
    const bucketRows = [...buckets.entries()].map(
      ([label, stats]): TrendDatum => ({
        Time: time,
        Channel: label,
        rawQuota: stats.quota,
        Usage: stats.quota
          ? Number((stats.quota / quotaPerUnit).toFixed(4))
          : 0,
        Count: stats.count,
        TimeSum: 0,
      })
    )
    bucketRows.sort((a, b) => b.Count - a.Count)
    requestTrendValues.push(...bucketRows)
  })
  requestTrendValues.sort((a, b) => a.Time.localeCompare(b.Time))

  // Cost ranking: top channels by total cost (Others merged).
  const costRankValues: RankDatum[] = costRanked
    .slice(0, topLimit)
    .map(([label, stats]) => ({
      Channel: label,
      rawQuota: stats.quota,
      Usage: stats.quota ? Number((stats.quota / quotaPerUnit).toFixed(4)) : 0,
      Count: stats.count,
    }))
  if (hasCostOverflow) {
    const otherQuota = costRanked
      .slice(topLimit)
      .reduce((sum, [, stats]) => sum + stats.quota, 0)
    costRankValues.push({
      Channel: otherLabel,
      rawQuota: otherQuota,
      Usage: otherQuota ? Number((otherQuota / quotaPerUnit).toFixed(4)) : 0,
      Count: 0,
    })
  }

  // Request ranking: top channels by total requests (Others merged).
  const requestRankValues: RankDatum[] = countRanked
    .slice(0, topLimit)
    .map(([label, stats]) => ({
      Channel: label,
      rawQuota: stats.quota,
      Usage: stats.quota ? Number((stats.quota / quotaPerUnit).toFixed(4)) : 0,
      Count: stats.count,
    }))
  if (hasCountOverflow) {
    const otherCount = countRanked
      .slice(topLimit)
      .reduce((sum, [, stats]) => sum + stats.count, 0)
    requestRankValues.push({
      Channel: otherLabel,
      rawQuota: 0,
      Usage: 0,
      Count: otherCount,
    })
  }

  return {
    spec_cost_trend: {
      type: 'bar',
      data: [{ id: 'costTrend', values: costTrendValues }],
      xField: 'Time',
      yField: 'Usage',
      seriesField: 'Channel',
      stack: true,
      legends: { visible: true, selectMode: 'single' },
      color,
      title: {
        visible: true,
        text: tt('Channel Cost Trend'),
        subtext: `${tt('Total:')} ${formatCost(totalQuota)}`,
      },
      bar: {
        state: { hover: { stroke: '#000', lineWidth: 1 } },
      },
      tooltip: makeCostTooltip(true),
      background: { fill: 'transparent' },
      animation: true,
    },
    spec_request_trend: {
      type: 'area',
      data: [{ id: 'requestTrend', values: requestTrendValues }],
      xField: 'Time',
      yField: 'Count',
      seriesField: 'Channel',
      stack: true,
      legends: { visible: true, selectMode: 'single' },
      color,
      title: {
        visible: true,
        text: tt('Channel Request Trend'),
        subtext: `${tt('Total:')} ${formatInt(totalCount)}`,
      },
      area: {
        style: { fillOpacity: 0.08, curveType: 'monotone' },
      },
      line: {
        style: { lineWidth: 2, curveType: 'monotone' },
      },
      point: { visible: false },
      tooltip: makeCountTooltip(),
      background: { fill: 'transparent' },
      animation: true,
    },
    spec_cost_rank: {
      type: 'bar',
      data: [{ id: 'costRank', values: costRankValues }],
      xField: 'rawQuota',
      yField: 'Channel',
      seriesField: 'Channel',
      direction: 'horizontal',
      legends: { visible: false },
      color: { specified: channelColor },
      title: {
        visible: true,
        text: tt('Channel Cost Ranking'),
        subtext: `${tt('Total:')} ${formatCost(totalQuota)}`,
      },
      bar: {
        state: { hover: { stroke: '#000', lineWidth: 1 } },
      },
      label: {
        visible: true,
        position: 'outside',
        formatMethod: (value: number) => formatCost(value),
        style: { fontSize: 11 },
      },
      axes: [
        { orient: 'left', type: 'band' },
        { orient: 'bottom', type: 'linear', visible: false },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Channel,
              value: (datum: Record<string, unknown>) =>
                formatCost(Number(datum?.rawQuota) || 0),
            },
          ],
        },
      },
      background: { fill: 'transparent' },
      animation: true,
    },
    spec_request_rank: {
      type: 'bar',
      data: [{ id: 'requestRank', values: requestRankValues }],
      xField: 'Count',
      yField: 'Channel',
      seriesField: 'Channel',
      direction: 'horizontal',
      legends: { visible: false },
      color: { specified: channelColor },
      title: {
        visible: true,
        text: tt('Channel Request Ranking'),
        subtext: `${tt('Total:')} ${formatInt(totalCount)}`,
      },
      bar: {
        state: { hover: { stroke: '#000', lineWidth: 1 } },
      },
      label: {
        visible: true,
        position: 'outside',
        formatMethod: (value: number) => formatInt(value),
        style: { fontSize: 11 },
      },
      axes: [
        { orient: 'left', type: 'band' },
        { orient: 'bottom', type: 'linear', visible: false },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Channel,
              value: (datum: Record<string, unknown>) =>
                formatInt(Number(datum?.Count) || 0),
            },
          ],
        },
      },
      background: { fill: 'transparent' },
      animation: true,
    },
    totalCostDisplay: formatCost(totalQuota),
    totalCountDisplay: formatInt(totalCount),
    totalTokenDisplay: formatInt(totalTokens),
  }
}
