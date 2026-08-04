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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

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
    assert.equal(groupFormSchema.safeParse(validValues).success, true)
  })

  test('rejects empty or special-character group names', () => {
    for (const name of ['', 'my group', 'vip:2', 'a,b', 'x"y']) {
      const result = groupFormSchema.safeParse({ ...validValues, name })
      assert.equal(result.success, false, `expected "${name}" to be rejected`)
    }
  })

  test('rejects negative base ratios', () => {
    const result = groupFormSchema.safeParse({ ...validValues, ratio: -1 })
    assert.equal(result.success, false)
  })

  test('rejects out-of-range rate limit values when enabled', () => {
    const result = groupFormSchema.safeParse({
      ...validValues,
      rateLimitEnabled: true,
      maxRequests: -1,
      maxSuccess: 0,
    })
    assert.equal(result.success, false)
  })

  test('accepts zero total requests when the rate limit is enabled', () => {
    const result = groupFormSchema.safeParse({
      ...validValues,
      rateLimitEnabled: true,
      maxRequests: 0,
      maxSuccess: 100,
    })
    assert.equal(result.success, true)
  })
})

describe('formValuesToRateLimit', () => {
  test('returns null when the rate limit is disabled', () => {
    assert.equal(
      formValuesToRateLimit({ ...validValues, rateLimitEnabled: false }),
      null
    )
  })

  test('returns the configured limits when enabled', () => {
    assert.deepEqual(
      formValuesToRateLimit({
        ...validValues,
        rateLimitEnabled: true,
        maxRequests: 200,
        maxSuccess: 100,
      }),
      { maxRequests: 200, maxSuccess: 100 }
    )
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
    assert.equal(values.name, 'vip')
    assert.equal(values.ratio, 0.8)
    assert.equal(values.topupRatio, '1.2')
    assert.equal(values.rateLimitEnabled, true)
    assert.equal(values.maxRequests, 100)
    assert.equal(values.maxSuccess, 50)
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

    assert.equal(values.rateLimitEnabled, false)
    assert.equal(values.maxRequests, 0)
    assert.equal(values.maxSuccess, 1)
  })
})
