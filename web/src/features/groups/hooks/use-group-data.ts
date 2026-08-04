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
import { toast } from 'sonner'

import { getGroupModels, getGroupOptions } from '../api'
import { parseGroupSettings } from '../lib/group-utils'
import type { GroupModelsResponse, GroupOptionsResponse } from '../types'

export function useGroupOptionsQuery(refreshTrigger = 0) {
  return useQuery({
    queryKey: ['group-options', refreshTrigger],
    queryFn: async () => {
      const result: GroupOptionsResponse = await getGroupOptions()
      if (!result.success) {
        toast.error(result.message || 'Failed to load group settings')
        return []
      }
      return result.data ?? []
    },
  })
}

export function useGroupSettingsQuery(refreshTrigger = 0) {
  const optionsQuery = useGroupOptionsQuery(refreshTrigger)
  return {
    ...optionsQuery,
    data: parseGroupSettings(optionsQuery.data ?? []),
  }
}

export function useGroupModelsQuery(refreshTrigger = 0) {
  return useQuery({
    queryKey: ['group-models', refreshTrigger],
    queryFn: async () => {
      const result: GroupModelsResponse = await getGroupModels()
      if (!result.success) {
        toast.error(result.message || 'Failed to load group models')
        return {}
      }
      return result.data ?? {}
    },
  })
}
