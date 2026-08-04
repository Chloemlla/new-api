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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Download, FileUp, Upload } from 'lucide-react'
import { type ChangeEvent, useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'

import { exportPricingConfig, importPricingConfig } from '../api'
import type { PricingConfigDocument } from '../types'

const MAX_IMPORT_FILE_BYTES = 10 * 1024 * 1024

function downloadJsonDocument(doc: PricingConfigDocument) {
  const blob = new Blob([JSON.stringify(doc, null, 2)], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `pricing-config-${new Date()
    .toISOString()
    .replaceAll(/[:.]/g, '-')}.json`
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  // Revoke on a later tick so the browser can start the download first.
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

function parseJsonObject(text: string): unknown {
  const parsed: unknown = JSON.parse(text)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('not-an-object')
  }
  return parsed
}

export function PricingImportExport() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [jsonText, setJsonText] = useState('')
  const [importOpen, setImportOpen] = useState(false)
  const [pendingPayload, setPendingPayload] = useState<unknown>(null)

  const exportMutation = useMutation({
    mutationFn: exportPricingConfig,
    onSuccess: (data) => {
      if (!data.success || !data.data) {
        toast.error(data.message || t('Failed to export pricing config'))
        return
      }
      downloadJsonDocument(data.data)
      toast.success(t('Pricing config exported successfully'))
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to export pricing config'))
    },
  })

  const importMutation = useMutation({
    mutationFn: importPricingConfig,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t('Pricing config imported successfully'))
        setJsonText('')
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
      } else {
        toast.error(data.message || t('Failed to import pricing config'))
      }
      setImportOpen(false)
      setPendingPayload(null)
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to import pricing config'))
      setImportOpen(false)
      setPendingPayload(null)
    },
  })

  const handleFileChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0]
      event.target.value = ''
      if (!file) return
      if (file.size > MAX_IMPORT_FILE_BYTES) {
        toast.error(t('Pricing config file is too large (max 10 MB)'))
        return
      }
      const reader = new FileReader()
      reader.addEventListener('load', () => {
        setJsonText(typeof reader.result === 'string' ? reader.result : '')
      })
      reader.readAsText(file)
    },
    [t]
  )

  const handleImport = useCallback(() => {
    let parsed: unknown
    try {
      parsed = parseJsonObject(jsonText)
    } catch {
      toast.error(t('Pricing config must be a JSON object'))
      return
    }
    setPendingPayload(parsed)
    setImportOpen(true)
  }, [jsonText, t])

  const handleConfirmImport = useCallback(() => {
    if (pendingPayload === null) return
    importMutation.mutate(pendingPayload)
  }, [importMutation, pendingPayload])

  const hasJson = jsonText.trim().length > 0
  const isBusy = exportMutation.isPending || importMutation.isPending

  return (
    <div className='grid gap-4 lg:grid-cols-2'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Export pricing config')}</CardTitle>
          <CardDescription>
            {t(
              'Download the current model pricing configuration as a JSON file that can be re-imported on this or another instance.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='text-muted-foreground text-sm'>
          {t(
            'The export includes model ratios, completion ratios, cache ratios, fixed prices, and tiered billing expressions.'
          )}
        </CardContent>
        <CardFooter className='justify-end'>
          <Button
            variant='outline'
            onClick={() => exportMutation.mutate()}
            disabled={isBusy}
          >
            <Download className='mr-2 h-4 w-4' />
            {exportMutation.isPending
              ? t('Exporting...')
              : t('Export pricing config')}
          </Button>
        </CardFooter>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Import pricing config')}</CardTitle>
          <CardDescription>
            {t(
              'Replace the current model pricing configuration from a JSON file. The imported values overwrite existing ones.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-3'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => fileInputRef.current?.click()}
            disabled={isBusy}
          >
            <FileUp className='mr-2 h-4 w-4' />
            {t('Select JSON file')}
          </Button>
          <input
            ref={fileInputRef}
            type='file'
            accept='.json,application/json'
            className='hidden'
            onChange={handleFileChange}
          />
          <Textarea
            value={jsonText}
            onChange={(event) => setJsonText(event.target.value)}
            placeholder={t('Paste exported pricing config JSON here...')}
            className='min-h-40 max-h-96 font-mono text-xs'
          />
        </CardContent>
        <CardFooter className='justify-end'>
          <Button
            onClick={handleImport}
            disabled={!hasJson || isBusy}
          >
            <Upload className='mr-2 h-4 w-4' />
            {importMutation.isPending
              ? t('Importing...')
              : t('Import pricing config')}
          </Button>
        </CardFooter>
      </Card>

      <ConfirmDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        title={t('Import pricing config?')}
        desc={t(
          'This will replace the current model pricing configuration with the imported values. This action cannot be undone.'
        )}
        destructive
        isLoading={importMutation.isPending}
        handleConfirm={handleConfirmImport}
        confirmText={t('Import')}
      />
    </div>
  )
}
