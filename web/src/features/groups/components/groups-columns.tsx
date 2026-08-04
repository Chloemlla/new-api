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
import type { ColumnDef } from '@tanstack/react-table'
import { Eye } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BadgeCell } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { LongText } from '@/components/long-text'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import type { UserGroup } from '../types'
import { DataTableRowActions } from './data-table-row-actions'

type GroupsColumnsProps = {
  onViewModels: (group: UserGroup) => void
}

export function useGroupsColumns(
  props: GroupsColumnsProps
): ColumnDef<UserGroup>[] {
  const { t } = useTranslation()
  const { onViewModels } = props

  return [
    {
      accessorKey: 'name',
      header: t('Group'),
      cell: ({ row }) => {
        const group = row.original
        return (
          <div className='flex min-w-[180px] flex-col gap-1'>
            <BadgeCell>
              <GroupBadge group={group.name} ratio={group.ratio} />
            </BadgeCell>
            {group.description && (
              <LongText className='text-muted-foreground max-w-[240px] text-xs'>
                {group.description}
              </LongText>
            )}
          </div>
        )
      },
      size: 240,
      meta: { mobileTitle: true },
    },
    {
      accessorKey: 'ratio',
      header: t('Ratio'),
      cell: ({ row }) => {
        const ratio = row.getValue('ratio') as number
        return <span className='font-mono text-sm tabular-nums'>{ratio}</span>
      },
      enableSorting: true,
      size: 80,
      meta: { mobileOrder: 20 },
    },
    {
      accessorKey: 'topupRatio',
      header: t('Top-up ratio'),
      cell: ({ row }) => {
        const value = row.getValue('topupRatio') as number | null
        return (
          <span className='text-muted-foreground text-sm'>
            {value == null ? t('Not set') : value}
          </span>
        )
      },
      size: 110,
      meta: { mobileOrder: 30 },
    },
    {
      accessorKey: 'selectable',
      header: t('User selectable'),
      cell: ({ row }) => {
        const selectable = row.getValue('selectable') as boolean
        return (
          <StatusBadge
            variant={selectable ? 'success' : 'neutral'}
            copyable={false}
          >
            {selectable ? t('Yes') : t('No')}
          </StatusBadge>
        )
      },
      enableSorting: false,
      size: 120,
      meta: { mobileOrder: 40 },
    },
    {
      id: 'rateLimit',
      header: t('Rate limit'),
      cell: ({ row }) => {
        const rateLimit = row.original.rateLimit
        if (!rateLimit) {
          return (
            <span className='text-muted-foreground text-sm'>
              {t('No limit')}
            </span>
          )
        }
        return (
          <StatusBadge variant='info' copyable={false}>
            {t('{{total}} req / {{success}} ok', {
              total: rateLimit.maxRequests,
              success: rateLimit.maxSuccess,
            })}
          </StatusBadge>
        )
      },
      enableSorting: false,
      size: 150,
      meta: { mobileOrder: 50 },
    },
    {
      id: 'models',
      header: t('Models'),
      cell: ({ row }) => {
        const group = row.original
        return (
          <Button
            variant='outline'
            size='sm'
            onClick={() => onViewModels(group)}
            aria-label={t('View models for {{group}}', { group: group.name })}
          >
            <Eye className='mr-1.5 h-3.5 w-3.5' />
            {group.modelCount}
          </Button>
        )
      },
      enableSorting: false,
      size: 100,
      meta: { mobileOrder: 60 },
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <DataTableRowActions row={row} />,
      meta: { pinned: 'right' as const },
    },
  ]
}
