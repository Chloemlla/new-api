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
import { useMutation } from '@tanstack/react-query'
import { Bell, Play, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import { runAlertCheck } from '../api'
import { useAlertRules } from './alert-rules-provider'

export function AlertRulesPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow, triggerRefresh } = useAlertRules()

  const checkMutation = useMutation({
    mutationFn: runAlertCheck,
    onSuccess: (result) => {
      if (result.success && result.data) {
        toast.success(
          t(
            'Alert check completed: {{rulesChecked}} rules checked, {{alertsSent}} alerts sent',
            {
              rulesChecked: result.data.rules_checked,
              alertsSent: result.data.alerts_sent,
            }
          )
        )
      } else {
        toast.error(result.message || t('Failed to run alert check'))
      }
      triggerRefresh()
    },
    onError: () => {
      toast.error(t('Failed to run alert check'))
    },
  })

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Button
        variant='outline'
        size='sm'
        onClick={() => {
          setCurrentRow(null)
          setOpen('test')
        }}
      >
        <Bell className='mr-2 h-4 w-4' />
        {t('Send Test Notification')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        onClick={() => checkMutation.mutate()}
        disabled={checkMutation.isPending}
      >
        <Play className='mr-2 h-4 w-4' />
        {checkMutation.isPending ? t('Checking...') : t('Check Now')}
      </Button>
      <Button
        size='sm'
        onClick={() => {
          setCurrentRow(null)
          setOpen('create')
        }}
      >
        <Plus className='mr-2 h-4 w-4' />
        {t('Create Alert Rule')}
      </Button>
    </div>
  )
}
