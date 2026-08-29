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

import type { UserGroup } from '../../types'
import {
  formValuesToRateLimit,
  groupFormSchema,
  groupToFormValues,
} from '../group-form'

const validValues = {
  name: 'premium',
  description: 'Premium plan',
  ratio: 0.5,
  topupRatio: '',
  selectable: true,
  rateLimitEnabled: false,
  maxRequests: 0,
  maxSuccess: 1,
}

describe('groupFormSchema', () => {
  test('accepts a valid group', () => {
    expect(groupFormSchema.safeParse(validValues).success).toBe(true)
  })

  test('rejects empty or special-character group names', () => {
    for (const name of ['', 'my group', 'vip:2', 'a,b', 'x"y']) {
      const result = groupFormSchema.safeParse({ ...validValues, name })
      expect(result.success, `expected "${name}" to be rejected`).toBe(false)
    }
  })

  test('rejects negative base ratios', () => {
    const result = groupFormSchema.safeParse({ ...validValues, ratio: -1 })
    expect(result.success).toBe(false)
  })

  test('rejects out-of-range rate limit values when enabled', () => {
    const result = groupFormSchema.safeParse({
      ...validValues,
      rateLimitEnabled: true,
      maxRequests: -1,
      maxSuccess: 0,
    })
    expect(result.success).toBe(false)
  })

  test('accepts zero total requests when the rate limit is enabled', () => {
    const result = groupFormSchema.safeParse({
      ...validValues,
      rateLimitEnabled: true,
      maxRequests: 0,
      maxSuccess: 100,
    })
    expect(result.success).toBe(true)
  })
})

describe('formValuesToRateLimit', () => {
  test('returns null when the rate limit is disabled', () => {
    expect(
      formValuesToRateLimit({ ...validValues, rateLimitEnabled: false })
    ).toBe(null)
  })

  test('returns the configured limits when enabled', () => {
    expect(
      formValuesToRateLimit({
        ...validValues,
        rateLimitEnabled: true,
        maxRequests: 200,
        maxSuccess: 100,
      })
    ).toEqual({ maxRequests: 200, maxSuccess: 100 })
  })
})

describe('groupToFormValues', () => {
  test('round-trips a group with a rate limit', () => {
    const group: UserGroup = {
      name: 'vip',
      description: 'VIP',
      ratio: 0.8,
      topupRatio: 1.2,
      selectable: true,
      rateLimit: { maxRequests: 100, maxSuccess: 50 },
      modelCount: 3,
    }

    const values = groupToFormValues(group)
    expect(values.name).toBe('vip')
    expect(values.ratio).toBe(0.8)
    expect(values.topupRatio).toBe('1.2')
    expect(values.rateLimitEnabled).toBe(true)
    expect(values.maxRequests).toBe(100)
    expect(values.maxSuccess).toBe(50)
  })

  test('maps a group without a rate limit to the disabled defaults', () => {
    const values = groupToFormValues({
      name: 'default',
      description: '',
      ratio: 1,
      topupRatio: null,
      selectable: true,
      rateLimit: null,
      modelCount: 0,
    })

    expect(values.rateLimitEnabled).toBe(false)
    expect(values.maxRequests).toBe(0)
    expect(values.maxSuccess).toBe(1)
  })
})
