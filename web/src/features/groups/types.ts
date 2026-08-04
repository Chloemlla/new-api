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
export type ApiResponse<T = unknown> = {
  success: boolean
  message: string
  data?: T
}

export type GroupRateLimit = [maxRequests: number, maxSuccess: number]

/**
 * Parsed view of a single user group, combining the group pricing options
 * (GroupRatio / TopupGroupRatio / UserUsableGroups) with the per-group
 * request rate limit and the number of enabled models.
 */
export type UserGroup = {
  name: string
  description: string
  ratio: number
  topupRatio: number | null
  selectable: boolean
  rateLimit: { maxRequests: number; maxSuccess: number } | null
  modelCount: number
}

/**
 * Normalized snapshot of every group-related system option. All JSON maps are
 * parsed into plain objects; `autoGroups` is a JSON array of group names.
 */
export type GroupSettings = {
  groupRatio: Record<string, number>
  userUsableGroups: Record<string, string>
  topupGroupRatio: Record<string, number>
  groupGroupRatio: Record<string, Record<string, number>>
  groupSpecialUsableGroup: Record<string, Record<string, string>>
  autoGroups: string[]
  rateLimitGroup: Record<string, GroupRateLimit>
}

export type GroupOptionsResponse = ApiResponse<{ key: string; value: string }[]>

export type GroupModelsResponse = ApiResponse<Record<string, string[]>>

export type UpdateOptionResponse = ApiResponse

export type GroupsDialogType = 'create' | 'update' | 'delete'
