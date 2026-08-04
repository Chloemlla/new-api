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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'

import {
  groupOperations,
  methodBadgeClass,
  operationAnchor,
} from '../lib/openapi'
import type { OpenAPISpec } from '../types'
import { Operation } from './operation'

export function ApiDoc({
  spec,
  baseUrl,
}: {
  spec: OpenAPISpec
  baseUrl?: string
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')

  const groups = useMemo(() => groupOperations(spec), [spec])
  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return groups
    return groups
      .map((group) => ({
        ...group,
        operations: group.operations.filter(
          (op) =>
            op.path.toLowerCase().includes(normalized) ||
            op.operation.summary?.toLowerCase().includes(normalized) ||
            op.operation.operationId?.toLowerCase().includes(normalized)
        ),
      }))
      .filter((group) => group.operations.length > 0)
  }, [groups, query])

  const hasFiltered = filtered.length > 0

  return (
    <div className='grid gap-8 lg:grid-cols-[280px_1fr]'>
      <aside className='hidden lg:block'>
        <div className='sticky top-24'>
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('Filter endpoints...')}
            className='mb-4'
          />
          <ScrollArea className='h-[calc(100vh-13rem)] pr-2'>
            <nav className='space-y-5'>
              {filtered.map((group) => (
                <div key={group.tag}>
                  <div className='text-muted-foreground mb-1.5 text-xs font-semibold tracking-wide uppercase'>
                    {group.tag}
                  </div>
                  <ul className='space-y-0.5'>
                    {group.operations.map(({ path, method }) => (
                      <li key={operationAnchor(method, path)}>
                        <a
                          href={`#${operationAnchor(method, path)}`}
                          className='hover:bg-accent focus:bg-accent text-muted-foreground hover:text-foreground flex items-center gap-2 rounded-md px-2 py-1 text-sm transition-colors focus:outline-none'
                        >
                          <span
                            className={cn(
                              'inline-block w-10 shrink-0 text-center font-mono text-[10px] font-bold uppercase',
                              methodBadgeClass(method)
                            )}
                          >
                            {method}
                          </span>
                          <span className='truncate font-mono text-xs'>
                            {path}
                          </span>
                        </a>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </nav>
          </ScrollArea>
        </div>
      </aside>

      <div className='min-w-0 space-y-10'>
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder={t('Filter endpoints...')}
          className='lg:hidden'
        />

        <header className='space-y-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <h1 className='text-2xl font-bold'>{spec.info.title}</h1>
            {spec.info.version && (
              <Badge variant='secondary' className='font-mono'>
                v{spec.info.version}
              </Badge>
            )}
          </div>
          {spec.info.description && (
            <Markdown className='text-muted-foreground'>
              {spec.info.description}
            </Markdown>
          )}
          {spec.servers && spec.servers.length > 0 && (
            <div className='flex flex-wrap items-center gap-2 text-sm'>
              <span className='text-muted-foreground'>{t('Base URL')}:</span>
              {spec.servers.map((server) => (
                <code
                  key={server.url}
                  className='bg-muted rounded px-1.5 py-0.5 font-mono text-xs'
                >
                  {server.url}
                </code>
              ))}
            </div>
          )}
          {!spec.servers?.length && baseUrl && (
            <div className='flex flex-wrap items-center gap-2 text-sm'>
              <span className='text-muted-foreground'>{t('Base URL')}:</span>
              <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-xs'>
                {baseUrl}
              </code>
            </div>
          )}
        </header>

        {query && !hasFiltered && (
          <p className='text-muted-foreground text-sm'>
            {t('No endpoints match your filter')}
          </p>
        )}

        {filtered.map((group) => (
          <section key={group.tag} className='space-y-4'>
            <div>
              <h2 className='text-xl font-semibold'>{group.tag}</h2>
              {group.tagDescription && (
                <Markdown className='text-muted-foreground'>
                  {group.tagDescription}
                </Markdown>
              )}
            </div>
            <div className='space-y-4'>
              {group.operations.map(({ path, method, operation }) => (
                <Operation
                  key={operationAnchor(method, path)}
                  spec={spec}
                  path={path}
                  method={method}
                  operation={operation}
                />
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  )
}
