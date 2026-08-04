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
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import { deleteAlertRule } from '../api'
import type { AlertRule } from '../types'
import { useAlertRules } from './alert-rules-provider'

type AlertRulesDeleteDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow: AlertRule | null
}

export function AlertRulesDeleteDialog({
  open,
  onOpenChange,
  currentRow,
}: AlertRulesDeleteDialogProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useAlertRules()

  const confirmDelete = async () => {
    if (!currentRow) return
    try {
      const result = await deleteAlertRule(currentRow.id)
      if (result.success) {
        toast.success(t('Alert rule deleted'))
        onOpenChange(false)
        triggerRefresh()
      }
    } catch {
      toast.error(t('Failed to delete alert rule'))
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('The alert rule "{{name}}" will be permanently deleted.', {
              name: currentRow?.name ?? '',
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
          <AlertDialogAction
            variant='destructive'
            onClick={() => void confirmDelete()}
          >
            {t('Delete')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
