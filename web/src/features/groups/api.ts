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
import { api } from '@/lib/api'

import type {
  GroupModelsResponse,
  GroupOptionsResponse,
  UpdateOptionResponse,
} from './types'

/**
 * Fetch every system option. The user group management page derives its group
 * list from the group pricing options (GroupRatio, UserUsableGroups,
 * TopupGroupRatio, ...) and the per-group rate limit map.
 */
export async function getGroupOptions(): Promise<GroupOptionsResponse> {
  const res = await api.get<GroupOptionsResponse>('/api/option/')
  return res.data
}

/**
 * Fetch the models enabled per user group (read-only, derived from the enabled
 * channel abilities).
 */
export async function getGroupModels(): Promise<GroupModelsResponse> {
  const res = await api.get<GroupModelsResponse>('/api/group/models')
  return res.data
}

/**
 * Update a single system option (JSON string value). Group pricing options are
 * stored as JSON strings in the options table and validated on the backend.
 */
export async function updateGroupOption(
  key: string,
  value: string
): Promise<UpdateOptionResponse> {
  const res = await api.put<UpdateOptionResponse>('/api/option/', {
    key,
    value,
  })
  return res.data
}
