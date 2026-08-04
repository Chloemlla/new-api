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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'

import {
  useGroupModelsQuery,
  useGroupSettingsQuery,
} from '../hooks/use-group-data'
import { buildUserGroups } from '../lib/group-utils'
import type { UserGroup } from '../types'
import { GroupModelsDialog } from './group-models-dialog'
import { useGroupsColumns } from './groups-columns'
import { useGroups } from './groups-provider'

export function GroupsTable() {
  const { t } = useTranslation()
  const { refreshTrigger } = useGroups()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [viewModelsGroup, setViewModelsGroup] = useState<UserGroup | null>(null)

  const settingsQuery = useGroupSettingsQuery(refreshTrigger)
  const modelsQuery = useGroupModelsQuery(refreshTrigger)

  const modelsByGroup = useMemo(
    () => modelsQuery.data ?? {},
    [modelsQuery.data]
  )

  const columns = useGroupsColumns({
    onViewModels: (group) => setViewModelsGroup(group),
  })

  const groups = useMemo(
    () => buildUserGroups(settingsQuery.data, modelsByGroup),
    [settingsQuery.data, modelsByGroup]
  )

  const { table } = useDataTable({
    data: groups,
    columns,
    initialPagination: { pageIndex: 0, pageSize: isMobile ? 10 : 20 },
    globalFilterFn: (row, _columnId, filterValue) => {
      const searchValue = String(filterValue).toLowerCase()
      const group = row.original
      return (
        group.name.toLowerCase().includes(searchValue) ||
        group.description.toLowerCase().includes(searchValue)
      )
    },
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={settingsQuery.isLoading || modelsQuery.isLoading}
        isFetching={settingsQuery.isFetching || modelsQuery.isFetching}
        emptyTitle={t('No Groups Found')}
        emptyDescription={t(
          'No user groups are configured yet. Add a group to get started.'
        )}
        skeletonKeyPrefix='groups-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t('Filter by group name or description...'),
          searchDebounceMs: 400,
        }}
      />

      <GroupModelsDialog
        open={viewModelsGroup !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setViewModelsGroup(null)
        }}
        group={viewModelsGroup}
        models={
          viewModelsGroup ? (modelsByGroup[viewModelsGroup.name] ?? []) : []
        }
      />
    </>
  )
}
