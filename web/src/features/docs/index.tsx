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
import { useQuery } from '@tanstack/react-query'
import { type TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  getOpenAPIDocs,
  getOpenAPISpec,
  type OpenAPIDocDescriptor,
} from './api'
import { ApiDoc } from './components/api-doc'

function docLabel(t: TFunction, id: string): string {
  if (id === 'relay') return t('Relay API')
  if (id === 'api') return t('Management API')
  return id
}

function DocLoading() {
  return (
    <div className='space-y-4'>
      <Skeleton className='h-8 w-64' />
      <Skeleton className='h-24 w-full' />
      <Skeleton className='h-24 w-full' />
      <Skeleton className='h-24 w-full' />
    </div>
  )
}

function DocTab({ doc }: { doc: OpenAPIDocDescriptor }) {
  const { t } = useTranslation()
  const {
    data: spec,
    isLoading,
    error,
  } = useQuery({
    queryKey: ['api-doc-spec', doc.id],
    queryFn: () => getOpenAPISpec(doc.url),
  })

  if (isLoading) return <DocLoading />
  if (error || !spec) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Failed to load documentation')}
      </p>
    )
  }
  return (
    <ApiDoc
      spec={spec}
      baseUrl={
        typeof window !== 'undefined' ? window.location.origin : undefined
      }
    />
  )
}

export function ApiDocs() {
  const { t } = useTranslation()
  const [selectedId, setSelectedId] = useState<string>()

  const {
    data: docs,
    isLoading,
    error,
  } = useQuery({
    queryKey: ['api-docs'],
    queryFn: getOpenAPIDocs,
  })

  const activeId = selectedId ?? docs?.[0]?.id

  return (
    <PublicLayout>
      <div className='space-y-6'>
        <div className='space-y-1'>
          <h1 className='text-2xl font-bold'>{t('API Documentation')}</h1>
          <p className='text-muted-foreground'>
            {t(
              'Browse the HTTP API reference for the relay and management endpoints.'
            )}
          </p>
        </div>

        {isLoading ? (
          <DocLoading />
        ) : error || !docs || docs.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('Unable to load documentation')}
          </p>
        ) : (
          <Tabs
            value={activeId ?? null}
            onValueChange={(value) => setSelectedId(value ?? undefined)}
          >
            <TabsList>
              {docs.map((doc) => (
                <TabsTrigger key={doc.id} value={doc.id}>
                  {docLabel(t, doc.id)}
                </TabsTrigger>
              ))}
            </TabsList>
            {docs.map((doc) => (
              <TabsContent key={doc.id} value={doc.id} className='pt-6'>
                <DocTab doc={doc} />
              </TabsContent>
            ))}
          </Tabs>
        )}
      </div>
    </PublicLayout>
  )
}
