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
import * as z from 'zod'

import type { UserGroup } from '../types'

/** Backend bound enforced by `CheckModelRequestRateLimitGroup`. */
export const MAX_RATE_LIMIT = 2147483647

export const groupFormSchema = z
  .object({
    name: z
      .string()
      .trim()
      .min(1, 'Group name is required')
      .max(64, 'Group name must be 64 characters or fewer')
      .regex(
        /^[^\s,:"']+$/,
        'Group name cannot contain spaces or special characters'
      ),
    description: z
      .string()
      .max(200, 'Description must be 200 characters or fewer'),
    ratio: z.number().min(0, 'Ratio must be 0 or greater'),
    topupRatio: z.string(),
    selectable: z.boolean(),
    rateLimitEnabled: z.boolean(),
    maxRequests: z.number().min(0).max(MAX_RATE_LIMIT),
    maxSuccess: z.number().min(1).max(MAX_RATE_LIMIT),
  })
  .superRefine((values, ctx) => {
    if (!values.rateLimitEnabled) return
    if (values.maxRequests < 0 || values.maxRequests > MAX_RATE_LIMIT) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['maxRequests'],
        message: `Must be between 0 and ${MAX_RATE_LIMIT.toLocaleString()}`,
      })
    }
    if (values.maxSuccess < 1 || values.maxSuccess > MAX_RATE_LIMIT) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['maxSuccess'],
        message: `Must be between 1 and ${MAX_RATE_LIMIT.toLocaleString()}`,
      })
    }
  })

export type GroupFormValues = z.infer<typeof groupFormSchema>

export const GROUP_FORM_DEFAULT_VALUES: GroupFormValues = {
  name: '',
  description: '',
  ratio: 1,
  topupRatio: '',
  selectable: true,
  rateLimitEnabled: false,
  maxRequests: 0,
  maxSuccess: 1,
}

export function groupToFormValues(group: UserGroup): GroupFormValues {
  return {
    name: group.name,
    description: group.description,
    ratio: group.ratio,
    topupRatio: group.topupRatio != null ? String(group.topupRatio) : '',
    selectable: group.selectable,
    rateLimitEnabled: group.rateLimit != null,
    maxRequests: group.rateLimit?.maxRequests ?? 0,
    maxSuccess: group.rateLimit?.maxSuccess ?? 1,
  }
}

export function formValuesToRateLimit(values: GroupFormValues): {
  maxRequests: number
  maxSuccess: number
} | null {
  if (!values.rateLimitEnabled) return null
  return { maxRequests: values.maxRequests, maxSuccess: values.maxSuccess }
}
