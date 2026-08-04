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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { updateGroupOption } from '../api'
import { useGroupSettingsQuery } from '../hooks/use-group-data'
import { applyGroupDelete, computeOptionUpdates } from '../lib/group-utils'
import { useGroups } from './groups-provider'

export function GroupDeleteDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh, refreshTrigger } =
    useGroups()
  const settingsQuery = useGroupSettingsQuery(refreshTrigger)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!currentRow) return

    setIsDeleting(true)
    try {
      const before = settingsQuery.data
      const after = applyGroupDelete(before, currentRow.name)
      const updates = computeOptionUpdates(before, after)
      for (const update of updates) {
        const result = await updateGroupOption(update.key, update.value)
        if (!result.success) {
          toast.error(result.message || t('Failed to delete group'))
          return
        }
      }
      toast.success(t('Group deleted successfully'))
      setOpen(null)
      triggerRefresh()
    } catch {
      toast.error(t('Failed to delete group'))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <ConfirmDialog
      open={open === 'delete'}
      onOpenChange={(isOpen) => !isOpen && setOpen(null)}
      title={t('Delete user group?')}
      desc={
        <>
          {t('This will remove the group')}{' '}
          <span className='font-semibold'>{currentRow?.name}</span>
          {t(
            ' from pricing, rate limits and group rules. Users and tokens assigned to it keep their group value but will no longer match any configured group. This action cannot be undone.'
          )}
        </>
      }
      confirmText={isDeleting ? t('Deleting...') : t('Delete')}
      destructive
      isLoading={isDeleting}
      handleConfirm={handleDelete}
    />
  )
}
