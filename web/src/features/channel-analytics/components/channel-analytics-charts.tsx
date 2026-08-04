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
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import {
  Activity,
  BarChart3,
  CircleAlert,
  Coins,
  ListOrdered,
  type LucideIcon,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { computeTimeRange } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { cn } from '@/lib/utils'
import { VCHART_OPTION } from '@/lib/vchart'

import { getChannelQuotaDates } from '../api'
import { TIME_RANGE_BY_GRANULARITY } from '../constants'
import { processChannelChartData } from '../lib/charts'
import type { ChannelAnalyticsFilters, VChartSpec } from '../types'

interface ChannelAnalyticsChartsProps {
  filters: ChannelAnalyticsFilters
  topLimit: number
}

function ChartCard(props: {
  title: string
  total: string
  icon: LucideIcon
  tone: IconBadgeTone
  loading: boolean
  themeReady: boolean
  theme: 'light' | 'dark'
  spec: VChartSpec
}) {
  const Icon = props.icon
  const specType = typeof props.spec?.type === 'string' ? props.spec.type : ''
  const valuesLength =
    Array.isArray(props.spec?.data) && Array.isArray(props.spec.data[0]?.values)
      ? props.spec.data[0].values.length
      : 0
  const chartKey = [
    props.title,
    specType,
    valuesLength,
    props.loading ? 'loading' : 'ready',
    props.theme,
  ].join('-')

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex min-w-0 items-center justify-between gap-2 border-b px-3 py-2 sm:px-4 sm:py-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <IconBadge tone={props.tone} size='sm'>
            <Icon />
          </IconBadge>
          <div className='truncate text-sm font-semibold'>{props.title}</div>
        </div>
        <span className='text-muted-foreground shrink-0 text-xs'>
          {props.total}
        </span>
      </div>
      <div className='h-[280px] p-1.5 sm:h-80 sm:p-2'>
        {props.loading ? (
          <Skeleton className='h-full w-full' />
        ) : (
          props.themeReady &&
          props.spec && (
            <VChart
              key={chartKey}
              spec={{
                ...props.spec,
                theme: props.theme,
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          )
        )}
      </div>
    </div>
  )
}

export function ChannelAnalyticsCharts(props: ChannelAnalyticsChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const granularity = props.filters.time_granularity
  const theme = resolvedTheme === 'dark' ? 'dark' : 'light'

  const timeRange = useMemo(
    () =>
      computeTimeRange(
        TIME_RANGE_BY_GRANULARITY[granularity],
        props.filters.start_timestamp,
        props.filters.end_timestamp
      ),
    [granularity, props.filters.start_timestamp, props.filters.end_timestamp]
  )

  const {
    data: rows,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ['channel-analytics', timeRange],
    queryFn: () =>
      getChannelQuotaDates({
        start_timestamp: timeRange.start_timestamp,
        end_timestamp: timeRange.end_timestamp,
      }),
    select: (res) => (res.success ? (res.data ?? []) : []),
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      processChannelChartData(
        isLoading ? [] : (rows ?? []),
        granularity,
        props.topLimit,
        t
      ),
    [isLoading, rows, granularity, props.topLimit, t]
  )

  const errorMessage =
    error instanceof Error ? error.message : t('Please try again later.')

  return (
    <div className='space-y-3 sm:space-y-4'>
      {isError && (
        <Alert variant='destructive'>
          <CircleAlert />
          <AlertTitle>{t('Failed to load')}</AlertTitle>
          <AlertDescription>{errorMessage}</AlertDescription>
        </Alert>
      )}

      <div className='divide-border/60 grid grid-cols-2 divide-x overflow-hidden rounded-lg border sm:grid-cols-3'>
        {[
          {
            title: t('Total Cost'),
            value: chartData.totalCostDisplay,
            tone: 'chart-1' as const,
          },
          {
            title: t('Requests'),
            value: chartData.totalCountDisplay,
            tone: 'chart-2' as const,
          },
          {
            title: t('Total Tokens'),
            value: chartData.totalTokenDisplay,
            tone: 'chart-3' as const,
          },
        ].map((item, index) => (
          <div
            key={item.title}
            className={cn(
              'min-w-0 px-3 py-2.5 sm:px-5 sm:py-4',
              index === 2 && 'col-span-2 sm:col-span-1'
            )}
          >
            <div className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
              {item.title}
            </div>
            <div className='text-foreground mt-1 truncate font-mono text-base leading-tight font-bold tracking-tight tabular-nums sm:text-2xl sm:leading-normal'>
              {isLoading ? <Skeleton className='h-6 w-20' /> : item.value}
            </div>
          </div>
        ))}
      </div>

      <div className='grid gap-3 sm:gap-4 md:grid-cols-2'>
        <ChartCard
          title={t('Channel Cost Trend')}
          total={`${t('Total:')} ${chartData.totalCostDisplay}`}
          icon={Coins}
          tone='chart-1'
          loading={isLoading}
          themeReady={themeReady}
          theme={theme}
          spec={chartData.spec_cost_trend}
        />
        <ChartCard
          title={t('Channel Request Trend')}
          total={`${t('Total:')} ${chartData.totalCountDisplay}`}
          icon={Activity}
          tone='chart-2'
          loading={isLoading}
          themeReady={themeReady}
          theme={theme}
          spec={chartData.spec_request_trend}
        />
        <ChartCard
          title={t('Channel Cost Ranking')}
          total={`${t('Total:')} ${chartData.totalCostDisplay}`}
          icon={BarChart3}
          tone='chart-3'
          loading={isLoading}
          themeReady={themeReady}
          theme={theme}
          spec={chartData.spec_cost_rank}
        />
        <ChartCard
          title={t('Channel Request Ranking')}
          total={`${t('Total:')} ${chartData.totalCountDisplay}`}
          icon={ListOrdered}
          tone='chart-4'
          loading={isLoading}
          themeReady={themeReady}
          theme={theme}
          spec={chartData.spec_request_rank}
        />
      </div>
    </div>
  )
}
