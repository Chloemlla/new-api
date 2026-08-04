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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import {
  refName,
  resolveRef,
  resolveSchema,
  schemaTypeName,
} from '../lib/openapi'
import type { OpenAPISchema, OpenAPISpec } from '../types'

type SchemaViewProps = {
  spec: OpenAPISpec
  schema?: OpenAPISchema
  depth?: number
  visitedRefs?: Set<string>
}

const MAX_DEPTH = 5

function formatValue(value: unknown): string {
  if (typeof value === 'string') return `"${value}"`
  return JSON.stringify(value)
}

function EnumList({ values }: { values: unknown[] }) {
  return (
    <span className='inline-flex flex-wrap items-center gap-1'>
      {values.map((value) => (
        <code
          key={formatValue(value)}
          className='bg-muted text-muted-foreground rounded px-1 py-0.5 font-mono text-[11px]'
        >
          {formatValue(value)}
        </code>
      ))}
    </span>
  )
}

function Constraint({ schema }: { schema: OpenAPISchema }) {
  const constraints: string[] = []
  if (schema.minimum !== undefined) {
    constraints.push(`≥ ${schema.minimum}`)
  }
  if (schema.maximum !== undefined) {
    constraints.push(`≤ ${schema.maximum}`)
  }
  if (schema.minItems !== undefined) {
    constraints.push(`min ${schema.minItems} items`)
  }
  if (schema.maxItems !== undefined) {
    constraints.push(`max ${schema.maxItems} items`)
  }
  if (schema.pattern) {
    constraints.push(`pattern ${schema.pattern}`)
  }
  if (constraints.length === 0) return null
  return (
    <span className='text-muted-foreground font-mono text-[11px]'>
      ({constraints.join(', ')})
    </span>
  )
}

function PropertyRow({
  spec,
  name,
  schema,
  required,
  depth,
  visitedRefs,
}: {
  spec: OpenAPISpec
  name: string
  schema: OpenAPISchema
  required: boolean
  depth: number
  visitedRefs: Set<string>
}) {
  const { t } = useTranslation()
  const resolved = resolveSchema(spec, schema)
  const expandable =
    !!resolved?.properties ||
    (resolved?.type === 'array' && !!resolved.items) ||
    !!resolved?.oneOf ||
    !!resolved?.anyOf ||
    !!resolved?.allOf

  return (
    <div className='px-3 py-2.5'>
      <div className='flex flex-wrap items-baseline gap-x-2 gap-y-1'>
        <span className='font-mono text-sm font-medium'>{name}</span>
        {required && (
          <Badge variant='destructive' className='h-4 px-1 text-[10px]'>
            {t('required')}
          </Badge>
        )}
        {schema.readOnly && (
          <Badge variant='outline' className='h-4 px-1 text-[10px]'>
            {t('read-only')}
          </Badge>
        )}
        <span className='text-muted-foreground font-mono text-[11px]'>
          {schemaTypeName(schema)}
        </span>
        <Constraint schema={schema} />
      </div>
      {(resolved?.description ?? schema.description) ? (
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {resolved?.description ?? schema.description}
        </p>
      ) : null}
      {schema.enum && (
        <div className='mt-1 flex flex-wrap items-center gap-1'>
          <span className='text-muted-foreground text-[11px]'>
            {t('enum')}:
          </span>
          <EnumList values={schema.enum} />
        </div>
      )}
      {schema.default !== undefined && (
        <div className='text-muted-foreground mt-1 text-[11px]'>
          {t('default')}:{' '}
          <code className='font-mono'>{formatValue(schema.default)}</code>
        </div>
      )}
      {expandable && depth < MAX_DEPTH && (
        <div className='mt-2'>
          <SchemaView
            spec={spec}
            schema={schema}
            depth={depth + 1}
            visitedRefs={visitedRefs}
          />
        </div>
      )}
    </div>
  )
}

export function SchemaView({
  spec,
  schema,
  depth = 0,
  visitedRefs,
}: SchemaViewProps) {
  const { t } = useTranslation()
  if (!schema) return null

  // $ref handling with cycle guard
  if (schema.$ref) {
    const name = refName(schema.$ref)
    if (visitedRefs?.has(name)) {
      return (
        <div className='text-muted-foreground text-xs'>
          <span className='font-mono'>{name}</span> ({t('Recursive reference')})
        </div>
      )
    }
    const resolved = resolveRef<OpenAPISchema>(spec, schema.$ref)
    if (!resolved) {
      return <span className='font-mono text-xs'>{name}</span>
    }
    return (
      <div>
        <div className='text-muted-foreground mb-1 text-xs'>
          <span className='font-mono'>{name}</span>
        </div>
        <SchemaView
          spec={spec}
          schema={resolved}
          depth={depth + 1}
          visitedRefs={new Set(visitedRefs ? [...visitedRefs, name] : [name])}
        />
      </div>
    )
  }

  const isObject = schema.type === 'object' || !!schema.properties
  if (isObject) {
    const properties = schema.properties ?? {}
    const required = new Set(schema.required ?? [])
    const names = Object.keys(properties)
    const hasAdditional = !!schema.additionalProperties
    const nextVisited = visitedRefs ?? new Set<string>()

    return (
      <div className='divide-border bg-card/40 divide-y overflow-hidden rounded-md border'>
        {names.length === 0 && !hasAdditional && (
          <div className='text-muted-foreground px-3 py-2 text-xs'>
            {t('Empty object')}
          </div>
        )}
        {names.map((name) => (
          <PropertyRow
            key={name}
            spec={spec}
            name={name}
            schema={properties[name]}
            required={required.has(name)}
            depth={depth}
            visitedRefs={nextVisited}
          />
        ))}
        {hasAdditional && (
          <div className='px-3 py-2.5'>
            <div className='flex flex-wrap items-baseline gap-x-2'>
              <span className='text-muted-foreground text-xs italic'>
                {t('additional properties')}
              </span>
              <span className='text-muted-foreground font-mono text-[11px]'>
                {schemaTypeName(
                  typeof schema.additionalProperties === 'object'
                    ? schema.additionalProperties
                    : undefined
                )}
              </span>
            </div>
          </div>
        )}
      </div>
    )
  }

  if (schema.type === 'array') {
    return (
      <div className='text-muted-foreground text-xs'>
        <span className='mr-1 font-medium'>{t('Array of')}:</span>
        <SchemaView
          spec={spec}
          schema={schema.items}
          depth={depth + 1}
          visitedRefs={visitedRefs}
        />
      </div>
    )
  }

  for (const [key, label] of [
    ['oneOf', t('One of')],
    ['anyOf', t('Any of')],
    ['allOf', t('All of')],
  ] as const) {
    const branches = schema[key]
    if (branches?.length) {
      return (
        <div className='space-y-2'>
          <div className='text-muted-foreground text-xs font-medium'>
            {label}:
          </div>
          {branches.map((branch, index) => (
            <div key={index} className='ml-2'>
              <SchemaView
                spec={spec}
                schema={branch}
                depth={depth + 1}
                visitedRefs={visitedRefs}
              />
            </div>
          ))}
        </div>
      )
    }
  }

  // Primitive
  return (
    <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
      <span className='text-muted-foreground font-mono text-[11px]'>
        {schemaTypeName(schema)}
      </span>
      <Constraint schema={schema} />
      {schema.enum && (
        <>
          <span className='text-muted-foreground text-[11px]'>
            {t('enum')}:
          </span>
          <EnumList values={schema.enum} />
        </>
      )}
      {schema.default !== undefined && (
        <span className='text-muted-foreground text-[11px]'>
          {t('default')}:{' '}
          <code className='font-mono'>{formatValue(schema.default)}</code>
        </span>
      )}
      {schema.example !== undefined && (
        <span className='text-muted-foreground text-[11px]'>
          {t('example')}:{' '}
          <code className='font-mono'>{formatValue(schema.example)}</code>
        </span>
      )}
    </div>
  )
}
