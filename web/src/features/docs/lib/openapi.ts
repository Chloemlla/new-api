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
import type { OpenAPIOperation, OpenAPISchema, OpenAPISpec } from '../types'

export type HttpMethod = 'get' | 'post' | 'put' | 'patch' | 'delete' | string

export const HTTP_METHODS: HttpMethod[] = [
  'get',
  'post',
  'put',
  'patch',
  'delete',
  'head',
  'options',
  'trace',
]

const METHOD_BADGE_CLASSES: Record<string, string> = {
  get: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
  post: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
  put: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
  patch: 'bg-violet-500/15 text-violet-600 dark:text-violet-400',
  delete: 'bg-red-500/15 text-red-600 dark:text-red-400',
}

export function methodBadgeClass(method: string): string {
  return (
    METHOD_BADGE_CLASSES[method.toLowerCase()] ??
    'bg-muted text-muted-foreground'
  )
}

const FALLBACK_TAG = 'Other'

export type GroupedOperation = {
  tag: string
  tagDescription?: string
  operations: {
    path: string
    method: string
    operation: OpenAPIOperation
  }[]
}

/**
 * Groups every path operation in the spec by its first tag, preserving the
 * tag order declared in the spec (unlisted tags appended afterwards).
 */
export function groupOperations(spec: OpenAPISpec): GroupedOperation[] {
  const groups = new Map<string, GroupedOperation>()

  for (const tag of spec.tags ?? []) {
    groups.set(tag.name, {
      tag: tag.name,
      tagDescription: tag.description,
      operations: [],
    })
  }

  for (const [path, item] of Object.entries(spec.paths)) {
    for (const method of HTTP_METHODS) {
      const operation = item[method]
      if (!operation || typeof operation !== 'object') continue

      const tag = operation.tags?.[0] ?? FALLBACK_TAG
      let group = groups.get(tag)
      if (!group) {
        group = { tag, operations: [] }
        groups.set(tag, group)
      }
      group.operations.push({ path, method, operation })
    }
  }

  return [...groups.values()].filter((g) => g.operations.length > 0)
}

/**
 * Resolves an OpenAPI $ref (e.g. "#/components/schemas/Message") against the
 * document. Returns undefined for unknown or non-$ref input.
 */
export function resolveRef<T>(spec: OpenAPISpec, ref?: string): T | undefined {
  if (!ref) return undefined
  const segments = ref.split('/')
  // Expected: ["#", "components", "schemas" | "responses" | "securitySchemes", name]
  if (segments[0] !== '#' || segments[1] !== 'components') return undefined

  const components = spec.components as Record<string, unknown> | undefined
  if (!components) return undefined
  const bucket = components[segments[2]] as Record<string, T> | undefined
  if (!bucket) return undefined
  return bucket[segments.slice(3).join('/')]
}

export function resolveSchema(
  spec: OpenAPISpec,
  schema?: OpenAPISchema
): OpenAPISchema | undefined {
  let current = schema
  const seen = new Set<string>()
  while (current?.$ref && !seen.has(current.$ref)) {
    seen.add(current.$ref)
    const resolved = resolveRef<OpenAPISchema>(spec, current.$ref)
    if (!resolved) return current
    current = resolved
  }
  return current
}

export function refName(ref?: string): string {
  if (!ref) return ''
  const parts = ref.split('/')
  for (let i = parts.length - 1; i >= 0; i--) {
    if (parts[i]) return parts[i]
  }
  return ''
}

/**
 * Stable anchor id for an operation used by the sidebar navigation links.
 */
export function operationAnchor(method: string, path: string): string {
  return `${method.toLowerCase()}-${path
    .replaceAll(/[^a-zA-Z0-9]+/g, '-')
    .replaceAll(/^-+|-+$/g, '')
    .slice(0, 60)}`
}

/**
 * Compact single-line description of a schema, e.g. "string", "integer",
 * "array<Message>", "object", "one of 2".
 */
export function schemaTypeName(schema?: OpenAPISchema): string {
  if (!schema) return 'any'
  if (schema.$ref) return refName(schema.$ref)

  switch (schema.type) {
    case 'array':
      return `array<${schemaTypeName(schema.items)}>`
    case 'object':
      return 'object'
    case 'string':
      return schema.format ? `string (${schema.format})` : 'string'
    case 'integer':
      return schema.format ? `integer (${schema.format})` : 'integer'
    case 'number':
      return schema.format ? `number (${schema.format})` : 'number'
    case 'boolean':
      return 'boolean'
    case 'null':
      return 'null'
  }
  if (schema.oneOf) return `one of ${schema.oneOf.length}`
  if (schema.anyOf) return `any of ${schema.anyOf.length}`
  if (schema.allOf) return `all of ${schema.allOf.length}`
  if (schema.properties) return 'object'
  return 'any'
}

export function firstMediaSchema(
  content?: Record<string, { schema?: OpenAPISchema }>
): { mediaType: string; schema?: OpenAPISchema } {
  if (!content) return { mediaType: '' }
  for (const mediaType of ['application/json', '*/*']) {
    if (content[mediaType]) {
      return { mediaType, schema: content[mediaType].schema }
    }
  }
  const first = Object.entries(content)[0]
  if (!first) return { mediaType: '' }
  return { mediaType: first[0], schema: first[1].schema }
}

export function statusBadgeClass(status: string): string {
  const code = Number.parseInt(status, 10)
  if (code >= 200 && code < 300) {
    return 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
  }
  if (code >= 400 && code < 500) {
    return 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
  }
  if (code >= 500) {
    return 'bg-red-500/15 text-red-600 dark:text-red-400'
  }
  return 'bg-muted text-muted-foreground'
}
