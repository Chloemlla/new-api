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
import { describe, expect, test } from 'vitest'

import type { GroupSettings } from '../../types'
import {
  applyGroupDelete,
  applyGroupUpsert,
  buildUserGroups,
  computeOptionUpdates,
  parseGroupSettings,
  serializeGroupSettings,
} from '../group-utils'

function makeSettings(overrides: Partial<GroupSettings> = {}): GroupSettings {
  return {
    groupRatio: { default: 1, vip: 0.8 },
    userUsableGroups: { default: 'Default group', vip: 'VIP group' },
    topupGroupRatio: { default: 1 },
    groupGroupRatio: { vip: { default: 0.9 } },
    groupSpecialUsableGroup: { vip: { '+:premium': 'Premium' } },
    autoGroups: ['default', 'vip'],
    rateLimitGroup: { default: [100, 50] },
    ...overrides,
  }
}

describe('parseGroupSettings', () => {
  test('parses JSON options into a settings snapshot', () => {
    const options = [
      { key: 'GroupRatio', value: '{"default":1,"vip":0.8}' },
      { key: 'UserUsableGroups', value: '{"default":"Default"}' },
      { key: 'TopupGroupRatio', value: '{"default":1}' },
      { key: 'GroupGroupRatio', value: '{"vip":{"default":0.9}}' },
      {
        key: 'group_ratio_setting.group_special_usable_group',
        value: '{"vip":{"+:premium":"Premium"}}',
      },
      { key: 'AutoGroups', value: '["default","vip"]' },
      { key: 'ModelRequestRateLimitGroup', value: '{"default":[100,50]}' },
    ]

    const settings = parseGroupSettings(options)

    expect(settings.groupRatio).toEqual({ default: 1, vip: 0.8 })
    expect(settings.userUsableGroups).toEqual({ default: 'Default' })
    expect(settings.topupGroupRatio).toEqual({ default: 1 })
    expect(settings.groupGroupRatio).toEqual({ vip: { default: 0.9 } })
    expect(settings.groupSpecialUsableGroup).toEqual({
      vip: { '+:premium': 'Premium' },
    })
    expect(settings.autoGroups).toEqual(['default', 'vip'])
    expect(settings.rateLimitGroup).toEqual({ default: [100, 50] })
  })

  test('falls back to empty defaults for missing or malformed options', () => {
    const settings = parseGroupSettings([
      { key: 'GroupRatio', value: 'not-json' },
    ])

    expect(settings.groupRatio).toEqual({})
    expect(settings.userUsableGroups).toEqual({})
    expect(settings.autoGroups).toEqual([])
  })
})

describe('buildUserGroups', () => {
  test('unions group names from every option and sorts them', () => {
    const settings = makeSettings({
      groupRatio: { vip: 0.8 },
      userUsableGroups: { premium: 'Premium' },
      topupGroupRatio: { default: 1 },
    })
    const groups = buildUserGroups(settings, { vip: ['gpt-4o'], default: [] })

    expect(groups.map((group) => group.name)).toEqual([
      'default',
      'premium',
      'vip',
    ])
  })

  test('fills defaults and attaches rate limits and model counts', () => {
    const settings = makeSettings({
      topupGroupRatio: {},
      rateLimitGroup: { default: [100, 50] },
    })
    const groups = buildUserGroups(settings, {
      default: ['gpt-4o', 'claude-3'],
      vip: ['gpt-4o-mini'],
    })

    const defaultGroup = groups.find((group) => group.name === 'default')
    expect(defaultGroup).toBeTruthy()
    expect(defaultGroup?.ratio).toBe(1)
    expect(defaultGroup?.topupRatio).toBe(null)
    expect(defaultGroup?.selectable).toBe(true)
    expect(defaultGroup?.description).toBe('Default group')
    expect(defaultGroup?.rateLimit).toEqual({
      maxRequests: 100,
      maxSuccess: 50,
    })
    expect(defaultGroup?.modelCount).toBe(2)

    const vipGroup = groups.find((group) => group.name === 'vip')
    expect(vipGroup).toBeTruthy()
    expect(vipGroup?.ratio).toBe(0.8)
    expect(vipGroup?.modelCount).toBe(1)
  })
})

describe('applyGroupUpsert', () => {
  test('creates a group across all group options', () => {
    const settings = makeSettings()
    const next = applyGroupUpsert(settings, {
      name: 'premium',
      description: 'Premium group',
      ratio: 0.5,
      topupRatio: '1.2',
      selectable: true,
      rateLimit: { maxRequests: 200, maxSuccess: 100 },
    })

    expect(next.groupRatio.premium).toBe(0.5)
    expect(next.userUsableGroups.premium).toBe('Premium group')
    expect(next.topupGroupRatio.premium).toBe(1.2)
    expect(next.rateLimitGroup.premium).toEqual([200, 100])
  })

  test('clears top-up ratio when left empty', () => {
    const settings = makeSettings({ topupGroupRatio: { default: 1, vip: 1 } })
    const next = applyGroupUpsert(settings, {
      name: 'vip',
      description: '',
      ratio: 0.8,
      topupRatio: '',
      selectable: false,
      rateLimit: null,
    })

    expect(Object.hasOwn(next.topupGroupRatio, 'vip')).toBe(false)
    expect(Object.hasOwn(next.userUsableGroups, 'vip')).toBe(false)
    expect(Object.hasOwn(next.rateLimitGroup, 'vip')).toBe(false)
  })
})

describe('applyGroupDelete', () => {
  test('removes the group from every group-related option', () => {
    const settings = makeSettings({
      groupGroupRatio: {
        vip: { default: 0.9, premium: 1.1 },
        premium: { default: 0.8 },
      },
      groupSpecialUsableGroup: {
        vip: { '+:premium': 'Premium' },
        premium: { '+:vip': 'VIP' },
      },
      autoGroups: ['default', 'vip', 'premium'],
      rateLimitGroup: { default: [100, 50], vip: [200, 100] },
    })

    const next = applyGroupDelete(settings, 'vip')

    expect(Object.hasOwn(next.groupRatio, 'vip')).toBe(false)
    expect(Object.hasOwn(next.userUsableGroups, 'vip')).toBe(false)
    expect(Object.hasOwn(next.topupGroupRatio, 'vip')).toBe(false)
    expect(Object.hasOwn(next.rateLimitGroup, 'vip')).toBe(false)
    expect(next.autoGroups).toEqual(['default', 'premium'])

    // vip removed as both a source and a target of overrides.
    expect(next.groupGroupRatio).toEqual({
      premium: { default: 0.8 },
    })
    expect(next.groupSpecialUsableGroup).toEqual({
      premium: { '+:vip': 'VIP' },
    })
  })
})

describe('computeOptionUpdates', () => {
  test('emits only the options that changed', () => {
    const before = makeSettings()
    const after = applyGroupUpsert(before, {
      name: 'premium',
      description: 'Premium',
      ratio: 0.5,
      topupRatio: '',
      selectable: true,
      rateLimit: null,
    })

    const updates = computeOptionUpdates(before, after)
    const keys = updates.map((update) => update.key)

    expect(keys).toEqual(['GroupRatio', 'UserUsableGroups'])
    expect(updates[0].value).toBe(JSON.stringify(after.groupRatio))
  })

  test('emits no updates when nothing changed', () => {
    const before = makeSettings()
    const after = makeSettings()
    expect(computeOptionUpdates(before, after)).toEqual([])
  })

  test('serialized settings are stable JSON for unchanged data', () => {
    const settings = makeSettings()
    expect(serializeGroupSettings(settings).GroupRatio).toBe(
      '{"default":1,"vip":0.8}'
    )
  })
})
