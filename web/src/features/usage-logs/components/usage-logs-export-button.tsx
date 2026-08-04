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
import { getRouteApi } from '@tanstack/react-router'
import { Download, Loader2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import { exportLogs } from '../api'
import { buildApiParams } from '../lib/utils'
import type { LogExportFormat } from '../types'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

/**
 * Extracts the server-side error message from a failed blob download. The
 * backend answers export errors with a non-2xx status and a JSON body, which
 * axios exposes as a Blob when `responseType: 'blob'` is set.
 */
async function readExportErrorMessage(error: unknown): Promise<string | null> {
  const data = (error as { response?: { data?: Blob } })?.response?.data
  if (!(data instanceof Blob)) return null
  const text = await data.text()
  try {
    const parsed = JSON.parse(text) as { message?: string }
    return parsed.message || null
  } catch {
    return text || null
  }
}

/**
 * Parses the filename out of the server's `Content-Disposition` header so the
 * downloaded file keeps the name the backend assigned to it.
 */
function serverFilename(disposition: unknown): string | null {
  if (typeof disposition !== 'string') return null
  const match = disposition.match(/filename="?([^";]+)"?/)
  return match ? match[1] : null
}

function triggerDownload(
  blob: Blob,
  format: LogExportFormat,
  disposition: unknown
) {
  const type = format === 'json' ? 'application/json' : 'text/csv'
  const url = URL.createObjectURL(new Blob([blob], { type }))
  const link = document.createElement('a')
  link.href = url
  link.download =
    serverFilename(disposition) ??
    `usage-logs-${new Date().toISOString().slice(0, 19).replaceAll(/[:T]/g, '-')}.${format}`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

/**
 * Page-header download control for the Common Logs view. Exports the logs
 * matching the currently applied URL filters as CSV or JSON, scoped to the
 * active view (all users for admins, otherwise the signed-in user).
 */
export function UsageLogsExportButton() {
  const { t } = useTranslation()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const searchParams = route.useSearch()
  const [exporting, setExporting] = useState<LogExportFormat | null>(null)

  const handleExport = useCallback(
    async (format: LogExportFormat) => {
      setExporting(format)
      try {
        const {
          p: _p,
          page_size: _pageSize,
          ...filters
        } = buildApiParams({
          page: 1,
          pageSize: 1,
          searchParams,
          columnFilters: [],
          isAdmin,
        })
        const res = await exportLogs({ ...filters, format }, isAdmin)
        triggerDownload(res.data, format, res.headers['content-disposition'])
        if (res.headers['x-export-truncated'] === 'true') {
          toast.warning(t('Export truncated'))
        }
      } catch (error) {
        const message = await readExportErrorMessage(error)
        toast.error(message || t('Export failed'))
      } finally {
        setExporting(null)
      }
    },
    [t, isAdmin, searchParams]
  )

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='outline'
            size='sm'
            disabled={exporting !== null}
            aria-label={t('Export')}
          >
            {exporting ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <Download className='size-4' />
            )}
            <span className='hidden sm:inline'>{t('Export')}</span>
          </Button>
        }
      />
      <DropdownMenuContent align='end'>
        <DropdownMenuLabel>{t('Export')}</DropdownMenuLabel>
        <DropdownMenuItem onClick={() => void handleExport('csv')}>
          {t('Export as CSV')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => void handleExport('json')}>
          {t('Export as JSON')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
