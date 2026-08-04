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
import type { GroupSettings, UserGroup } from '../types'

export const GROUP_RATIO_KEY = 'GroupRatio'
export const USER_USABLE_GROUPS_KEY = 'UserUsableGroups'
export const TOPUP_GROUP_RATIO_KEY = 'TopupGroupRatio'
export const GROUP_GROUP_RATIO_KEY = 'GroupGroupRatio'
export const GROUP_SPECIAL_USABLE_KEY =
  'group_ratio_setting.group_special_usable_group'
export const AUTO_GROUPS_KEY = 'AutoGroups'
export const RATE_LIMIT_GROUP_KEY = 'ModelRequestRateLimitGroup'

export type OptionUpdate = { key: string; value: string }

/**
 * Best-effort JSON parse with a fallback value. Group options are stored as
 * JSON strings; a malformed entry must never crash the management page.
 */
export function safeJsonParse<T>(
  value: string | undefined,
  fallback: T,
  context?: string
): T {
  if (!value || value.trim() === '') return fallback
  try {
    const parsed = JSON.parse(value) as T
    return parsed ?? fallback
  } catch {
    if (import.meta.env.DEV) {
      // eslint-disable-next-line no-console
      console.error(`[Group settings] invalid ${context ?? 'JSON'}`, value)
    }
    return fallback
  }
}

function readOption(
  options: { key: string; value: string }[],
  key: string
): string {
  return options.find((option) => option.key === key)?.value ?? ''
}

/**
 * Build a normalized GroupSettings snapshot from the raw `/api/option/` list.
 */
export function parseGroupSettings(
  options: { key: string; value: string }[]
): GroupSettings {
  return {
    groupRatio: safeJsonParse<Record<string, number>>(
      readOption(options, GROUP_RATIO_KEY),
      {},
      'group ratio'
    ),
    userUsableGroups: safeJsonParse<Record<string, string>>(
      readOption(options, USER_USABLE_GROUPS_KEY),
      {},
      'user usable groups'
    ),
    topupGroupRatio: safeJsonParse<Record<string, number>>(
      readOption(options, TOPUP_GROUP_RATIO_KEY),
      {},
      'topup group ratio'
    ),
    groupGroupRatio: safeJsonParse<Record<string, Record<string, number>>>(
      readOption(options, GROUP_GROUP_RATIO_KEY),
      {},
      'group group ratio'
    ),
    groupSpecialUsableGroup: safeJsonParse<
      Record<string, Record<string, string>>
    >(
      readOption(options, GROUP_SPECIAL_USABLE_KEY),
      {},
      'special usable group'
    ),
    autoGroups: safeJsonParse<string[]>(
      readOption(options, AUTO_GROUPS_KEY),
      [],
      'auto groups'
    ),
    rateLimitGroup: safeJsonParse<Record<string, [number, number]>>(
      readOption(options, RATE_LIMIT_GROUP_KEY),
      {},
      'rate limit group'
    ),
  }
}

/**
 * Derive the displayable group list. The group pool is the union of every
 * group referenced by a group pricing option or the per-group rate limit map.
 */
export function buildUserGroups(
  settings: GroupSettings,
  modelsByGroup: Record<string, string[]>
): UserGroup[] {
  const names = new Set([
    ...Object.keys(settings.groupRatio),
    ...Object.keys(settings.userUsableGroups),
    ...Object.keys(settings.topupGroupRatio),
    ...Object.keys(settings.rateLimitGroup),
  ])

  return [...names]
    .sort((a, b) => a.localeCompare(b))
    .map((name) => {
      const rateLimit = settings.rateLimitGroup[name]
      return {
        name,
        description: settings.userUsableGroups[name] ?? '',
        ratio: settings.groupRatio[name] ?? 1,
        topupRatio: Object.hasOwn(settings.topupGroupRatio, name)
          ? settings.topupGroupRatio[name]
          : null,
        selectable: Object.hasOwn(settings.userUsableGroups, name),
        rateLimit: rateLimit
          ? { maxRequests: rateLimit[0], maxSuccess: rateLimit[1] }
          : null,
        modelCount: modelsByGroup[name]?.length ?? 0,
      }
    })
}

/**
 * Apply a create/update of a single group to a settings snapshot. The group
 * name is treated as immutable during updates (handled by the caller); here it
 * always writes into the target name key.
 */
export function applyGroupUpsert(
  settings: GroupSettings,
  input: {
    name: string
    description: string
    ratio: number
    topupRatio: string
    selectable: boolean
    rateLimit: { maxRequests: number; maxSuccess: number } | null
  }
): GroupSettings {
  const name = input.name.trim()

  const groupRatio = { ...settings.groupRatio, [name]: input.ratio }

  const topupGroupRatio = { ...settings.topupGroupRatio }
  const rawTopup = input.topupRatio.trim()
  if (rawTopup !== '' && Number.isFinite(Number(rawTopup))) {
    topupGroupRatio[name] = Number(rawTopup)
  } else {
    delete topupGroupRatio[name]
  }

  const userUsableGroups = { ...settings.userUsableGroups }
  if (input.selectable) {
    userUsableGroups[name] = input.description
  } else {
    delete userUsableGroups[name]
  }

  const rateLimitGroup = { ...settings.rateLimitGroup }
  if (input.rateLimit) {
    rateLimitGroup[name] = [
      input.rateLimit.maxRequests,
      input.rateLimit.maxSuccess,
    ]
  } else {
    delete rateLimitGroup[name]
  }

  return {
    ...settings,
    groupRatio,
    topupGroupRatio,
    userUsableGroups,
    rateLimitGroup,
  }
}

/**
 * Remove a group from every group-related option: pricing tables, descriptions,
 * rate limits, inter-group overrides (as both source and target), special
 * usable rules and the auto assignment order.
 */
export function applyGroupDelete(
  settings: GroupSettings,
  name: string
): GroupSettings {
  const groupRatio = { ...settings.groupRatio }
  delete groupRatio[name]

  const userUsableGroups = { ...settings.userUsableGroups }
  delete userUsableGroups[name]

  const topupGroupRatio = { ...settings.topupGroupRatio }
  delete topupGroupRatio[name]

  const rateLimitGroup = { ...settings.rateLimitGroup }
  delete rateLimitGroup[name]

  const groupGroupRatio: Record<string, Record<string, number>> = {}
  for (const [userGroup, overrides] of Object.entries(
    settings.groupGroupRatio
  )) {
    if (userGroup === name) continue
    const nextOverrides: Record<string, number> = {}
    for (const [targetGroup, ratio] of Object.entries(overrides)) {
      if (targetGroup === name) continue
      nextOverrides[targetGroup] = ratio
    }
    if (Object.keys(nextOverrides).length > 0) {
      groupGroupRatio[userGroup] = nextOverrides
    }
  }

  const groupSpecialUsableGroup = { ...settings.groupSpecialUsableGroup }
  delete groupSpecialUsableGroup[name]

  const autoGroups = settings.autoGroups.filter((group) => group !== name)

  return {
    ...settings,
    groupRatio,
    userUsableGroups,
    topupGroupRatio,
    rateLimitGroup,
    groupGroupRatio,
    groupSpecialUsableGroup,
    autoGroups,
  }
}

const ALL_OPTION_KEYS = [
  GROUP_RATIO_KEY,
  USER_USABLE_GROUPS_KEY,
  TOPUP_GROUP_RATIO_KEY,
  GROUP_GROUP_RATIO_KEY,
  GROUP_SPECIAL_USABLE_KEY,
  AUTO_GROUPS_KEY,
  RATE_LIMIT_GROUP_KEY,
] as const

/**
 * Serialize a settings snapshot to the option key → JSON string payloads used
 * by `/api/option/`. Group options are stored as JSON strings on the backend.
 */
export function serializeGroupSettings(
  settings: GroupSettings
): Record<string, string> {
  return {
    [GROUP_RATIO_KEY]: JSON.stringify(settings.groupRatio),
    [USER_USABLE_GROUPS_KEY]: JSON.stringify(settings.userUsableGroups),
    [TOPUP_GROUP_RATIO_KEY]: JSON.stringify(settings.topupGroupRatio),
    [GROUP_GROUP_RATIO_KEY]: JSON.stringify(settings.groupGroupRatio),
    [GROUP_SPECIAL_USABLE_KEY]: JSON.stringify(
      settings.groupSpecialUsableGroup
    ),
    [AUTO_GROUPS_KEY]: JSON.stringify(settings.autoGroups),
    [RATE_LIMIT_GROUP_KEY]: JSON.stringify(settings.rateLimitGroup),
  }
}

/**
 * Compute the list of option updates needed to transition from `before` to
 * `after`. Only changed options are emitted so saves stay minimal.
 */
export function computeOptionUpdates(
  before: GroupSettings,
  after: GroupSettings
): OptionUpdate[] {
  const beforeSerialized = serializeGroupSettings(before)
  const afterSerialized = serializeGroupSettings(after)
  return ALL_OPTION_KEYS.filter(
    (key) => beforeSerialized[key] !== afterSerialized[key]
  ).map((key) => ({ key, value: afterSerialized[key] }))
}
