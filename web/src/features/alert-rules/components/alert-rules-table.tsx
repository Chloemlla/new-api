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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { StatusBadge } from '@/components/status-badge'
import dayjs from '@/lib/dayjs'

import { getAlertRules } from '../api'
import {
  ERROR_MESSAGES,
  scopeLabelKey,
  triggerTypeLabelKey,
} from '../constants'
import type { AlertRule } from '../types'
import { useAlertRules } from './alert-rules-provider'

export function AlertRulesTable() {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow, refreshTrigger } = useAlertRules()

  const { data, isLoading } = useQuery({
    queryKey: ['alert-rules', refreshTrigger],
    queryFn: async () => {
      const result = await getAlertRules()
      if (!result.success) {
        toast.error(result.message || t(ERROR_MESSAGES.LOAD_FAILED))
        return []
      }
      return result.data ?? []
    },
  })

  const rules = data ?? []

  const renderThreshold = (rule: AlertRule) =>
    rule.trigger_type === 'channel_failure_rate'
      ? t('> {{threshold}}%', { threshold: rule.threshold })
      : t('< ${{threshold}}', { threshold: rule.threshold })

  const renderNotificationTargets = (rule: AlertRule) => {
    const targets: string[] = []
    if (rule.webhook_url !== '') targets.push(t('Webhook'))
    if (rule.email !== '') targets.push(t('Email'))
    return targets.length > 0 ? targets.join(' + ') : '-'
  }

  return (
    <StaticDataTable
      data={rules}
      getRowKey={(rule) => rule.id}
      empty={rules.length === 0}
      emptyContent={
        isLoading
          ? t('Loading...')
          : t(
              'No alert rules configured yet. Click "Create Alert Rule" to add one.'
            )
      }
      columns={[
        {
          id: 'name',
          header: t('Name'),
          cell: (rule) => (
            <div className='flex items-center gap-2'>
              <span className='font-medium'>{rule.name}</span>
              {rule.enabled ? (
                <StatusBadge
                  label={t('Enabled')}
                  variant='success'
                  copyable={false}
                />
              ) : (
                <StatusBadge
                  label={t('Disabled')}
                  variant='neutral'
                  copyable={false}
                />
              )}
            </div>
          ),
        },
        {
          id: 'trigger',
          header: t('Trigger'),
          cell: (rule) => (
            <div className='flex flex-col gap-0.5'>
              <span className='text-sm font-medium'>
                {t(triggerTypeLabelKey(rule))}
              </span>
              <span className='text-muted-foreground text-xs'>
                {renderThreshold(rule)}
              </span>
            </div>
          ),
        },
        {
          id: 'scope',
          header: t('Scope'),
          cell: (rule) => {
            if (rule.scope === 'tag') return rule.channel_tag || '-'
            if (rule.scope === 'ids') return rule.channel_ids.join(', ')
            return t(scopeLabelKey(rule))
          },
        },
        {
          id: 'notification',
          header: t('Notification'),
          cell: (rule) => (
            <div className='flex flex-col gap-0.5'>
              <span className='text-sm'>{renderNotificationTargets(rule)}</span>
              {rule.cooldown_minutes > 0 ? (
                <span className='text-muted-foreground text-xs'>
                  {t('cooldown: {{minutes}}m', {
                    minutes: rule.cooldown_minutes,
                  })}
                </span>
              ) : null}
            </div>
          ),
        },
        {
          id: 'last_triggered_at',
          header: t('Last Triggered'),
          cell: (rule) =>
            rule.last_triggered_at > 0
              ? dayjs(rule.last_triggered_at * 1000).format('YYYY-MM-DD HH:mm')
              : '-',
        },
        {
          id: 'actions',
          header: t('Actions'),
          cell: (rule) => (
            <StaticRowActions
              editLabel={t('Edit')}
              deleteLabel={t('Delete')}
              menuLabel={t('Open menu')}
              onEdit={() => {
                setCurrentRow(rule)
                setOpen('update')
              }}
              onDelete={() => {
                setCurrentRow(rule)
                setOpen('delete')
              }}
            />
          ),
        },
      ]}
    />
  )
}
