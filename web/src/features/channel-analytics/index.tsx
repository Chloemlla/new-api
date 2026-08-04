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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { getRollingDateRange } from '@/lib/time'

import { ChannelAnalyticsCharts } from './components/channel-analytics-charts'
import { ChannelAnalyticsFilterDialog } from './components/channel-analytics-filter-dialog'
import {
  DEFAULT_CHANNEL_ANALYTICS_GRANULARITY,
  DEFAULT_CHANNEL_ANALYTICS_TOP_LIMIT,
  TIME_RANGE_BY_GRANULARITY,
} from './constants'
import type { ChannelAnalyticsFilters } from './types'

function buildDefaultChannelAnalyticsFilters(): ChannelAnalyticsFilters {
  const granularity = DEFAULT_CHANNEL_ANALYTICS_GRANULARITY
  const { start, end } = getRollingDateRange(
    TIME_RANGE_BY_GRANULARITY[granularity]
  )
  return {
    start_timestamp: start,
    end_timestamp: end,
    time_granularity: granularity,
  }
}

export function ChannelAnalytics() {
  const { t } = useTranslation()
  const [filters, setFilters] = useState<ChannelAnalyticsFilters>(() =>
    buildDefaultChannelAnalyticsFilters()
  )
  const [topLimit, setTopLimit] = useState(DEFAULT_CHANNEL_ANALYTICS_TOP_LIMIT)

  const handleApply = (
    nextFilters: ChannelAnalyticsFilters,
    nextTopLimit: number
  ) => {
    setFilters(nextFilters)
    setTopLimit(nextTopLimit)
  }

  const handleReset = () => {
    setFilters(buildDefaultChannelAnalyticsFilters())
    setTopLimit(DEFAULT_CHANNEL_ANALYTICS_TOP_LIMIT)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Channel Cost Analytics')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <ChannelAnalyticsFilterDialog
          filters={filters}
          topLimit={topLimit}
          onApply={handleApply}
          onReset={handleReset}
        />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <ChannelAnalyticsCharts filters={filters} topLimit={topLimit} />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
