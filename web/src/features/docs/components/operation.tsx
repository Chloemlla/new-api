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
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Markdown } from '@/components/ui/markdown'

import {
  firstMediaSchema,
  methodBadgeClass,
  operationAnchor,
  resolveRef,
  schemaTypeName,
  statusBadgeClass,
} from '../lib/openapi'
import type {
  OpenAPIOperation,
  OpenAPIParameter,
  OpenAPIResponse,
  OpenAPISecurityScheme,
  OpenAPISpec,
} from '../types'
import { SchemaView } from './schema-view'

type OperationProps = {
  spec: OpenAPISpec
  path: string
  method: string
  operation: OpenAPIOperation
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h4 className='text-muted-foreground text-xs font-semibold tracking-wide uppercase'>
      {children}
    </h4>
  )
}

function ParameterList({
  spec,
  parameters,
}: {
  spec: OpenAPISpec
  parameters: OpenAPIParameter[]
}) {
  const { t } = useTranslation()
  return (
    <div className='divide-border bg-card/40 divide-y overflow-hidden rounded-md border'>
      {parameters.map((parameter, index) => (
        <div key={index} className='px-3 py-2.5'>
          <div className='flex flex-wrap items-baseline gap-x-2 gap-y-1'>
            <span className='font-mono text-sm font-medium'>
              {parameter.name}
            </span>
            <Badge variant='secondary' className='h-4 px-1 text-[10px]'>
              {parameter.in}
            </Badge>
            {parameter.required && (
              <Badge variant='destructive' className='h-4 px-1 text-[10px]'>
                {t('required')}
              </Badge>
            )}
            <span className='text-muted-foreground font-mono text-[11px]'>
              {schemaTypeName(parameter.schema)}
            </span>
          </div>
          {parameter.description && (
            <p className='text-muted-foreground mt-0.5 text-xs'>
              {parameter.description}
            </p>
          )}
        </div>
      ))}
    </div>
  )
}

function SecurityInfo({
  spec,
  operation,
}: {
  spec: OpenAPISpec
  operation: OpenAPIOperation
}) {
  const { t } = useTranslation()
  const requirements = operation.security ?? spec.security
  if (!requirements || requirements.length === 0) return null

  const schemes = new Set<string>()
  for (const requirement of requirements) {
    for (const name of Object.keys(requirement)) {
      schemes.add(name)
    }
  }
  if (schemes.size === 0) return null

  const describe = (name: string): string => {
    const scheme = resolveRef<OpenAPISecurityScheme>(
      spec,
      `#/components/securitySchemes/${name}`
    )
    if (!scheme) return name
    if (scheme.type === 'http') {
      const parts = [`HTTP ${scheme.scheme ?? 'auth'}`]
      if (scheme.bearerFormat) parts.push(scheme.bearerFormat)
      return parts.join(' · ')
    }
    if (scheme.type === 'apiKey') {
      return `API key (${scheme.name ?? name}${scheme.in ? `, ${scheme.in}` : ''})`
    }
    return name
  }

  return (
    <div className='flex flex-wrap items-center gap-1.5'>
      <span className='text-muted-foreground text-xs'>
        {t('Authentication')}:
      </span>
      {[...schemes].map((name) => (
        <Badge key={name} variant='outline' className='font-mono'>
          {describe(name)}
        </Badge>
      ))}
    </div>
  )
}

export function Operation({ spec, path, method, operation }: OperationProps) {
  const { t } = useTranslation()
  const parameters = operation.parameters ?? []
  const requestBody = operation.requestBody
  const bodySchema = firstMediaSchema(requestBody?.content)
  const responses = operation.responses ?? {}

  return (
    <Card id={operationAnchor(method, path)} className='scroll-mt-24'>
      <CardHeader className='pb-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <span
            className={`inline-flex h-5 min-w-11 items-center justify-center rounded-md px-1.5 font-mono text-[11px] font-bold tracking-wide uppercase ${methodBadgeClass(method)}`}
          >
            {method}
          </span>
          <code className='font-mono text-sm'>{path}</code>
          {operation.deprecated && (
            <Badge variant='warning'>{t('Deprecated')}</Badge>
          )}
        </div>
        {operation.summary && (
          <CardTitle className='text-base'>{operation.summary}</CardTitle>
        )}
        {operation.description ? (
          <CardDescription>
            <Markdown>{operation.description}</Markdown>
          </CardDescription>
        ) : null}
        <SecurityInfo spec={spec} operation={operation} />
      </CardHeader>

      <CardContent className='space-y-6'>
        {parameters.length > 0 && (
          <div className='space-y-2'>
            <SectionTitle>{t('Parameters')}</SectionTitle>
            <ParameterList spec={spec} parameters={parameters} />
          </div>
        )}

        {bodySchema.schema && (
          <div className='space-y-2'>
            <SectionTitle>
              {t('Request Body')}
              {requestBody?.required ? ` (${t('Required')})` : ''}
            </SectionTitle>
            <div className='text-muted-foreground text-xs'>
              <code className='font-mono'>{bodySchema.mediaType}</code>
            </div>
            <SchemaView spec={spec} schema={bodySchema.schema} />
          </div>
        )}

        <div className='space-y-2'>
          <SectionTitle>{t('Responses')}</SectionTitle>
          <div className='space-y-3'>
            {Object.entries(responses).map(([status, rawResponse]) => {
              if (!rawResponse) return null
              const response =
                resolveRef<OpenAPIResponse>(spec, rawResponse.$ref) ??
                rawResponse
              const responseSchema = firstMediaSchema(response.content)
              return (
                <div key={status} className='space-y-1.5'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <span
                      className={`inline-flex h-5 min-w-11 items-center justify-center rounded-md px-1.5 font-mono text-[11px] font-bold ${statusBadgeClass(status)}`}
                    >
                      {status}
                    </span>
                    {response.description && (
                      <span className='text-muted-foreground text-sm'>
                        {response.description}
                      </span>
                    )}
                    {responseSchema.schema && (
                      <span className='text-muted-foreground font-mono text-[11px]'>
                        {schemaTypeName(responseSchema.schema)}
                      </span>
                    )}
                  </div>
                  {responseSchema.schema && (
                    <div className='ml-0'>
                      <SchemaView spec={spec} schema={responseSchema.schema} />
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
