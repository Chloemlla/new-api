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
import { Box } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import type { UserGroup } from '../types'

type GroupModelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  group: UserGroup | null
  models: string[]
}

export function GroupModelsDialog(props: GroupModelsDialogProps) {
  const { t } = useTranslation()
  const { open, onOpenChange, group, models } = props

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        group ? t('Models for {{group}}', { group: group.name }) : t('Models')
      }
      description={t(
        'Models this group can use, based on the enabled channels that serve the group. Model access is controlled by channel configuration.'
      )}
      contentClassName='sm:max-w-[560px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <Button
          type='button'
          variant='outline'
          onClick={() => onOpenChange(false)}
        >
          {t('Close')}
        </Button>
      }
    >
      {models.length === 0 ? (
        <div className='text-muted-foreground flex h-24 items-center justify-center gap-2 text-sm'>
          <Box className='h-4 w-4' />
          {t('No models are enabled for this group.')}
        </div>
      ) : (
        <div className='flex max-h-72 flex-wrap gap-2 overflow-y-auto'>
          {models.map((modelName) => (
            <Badge key={modelName} variant='outline' className='font-mono'>
              {modelName}
            </Badge>
          ))}
        </div>
      )}
    </Dialog>
  )
}
