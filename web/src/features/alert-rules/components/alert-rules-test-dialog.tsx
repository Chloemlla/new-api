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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { testAlertRule } from '../api'

type TestFormValues = {
  webhook_url: string
  webhook_secret: string
  email: string
}

type AlertRulesTestDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AlertRulesTestDialog({
  open,
  onOpenChange,
}: AlertRulesTestDialogProps) {
  const { t } = useTranslation()
  const [isSending, setIsSending] = useState(false)
  const form = useForm<TestFormValues>({
    defaultValues: {
      webhook_url: '',
      webhook_secret: '',
      email: '',
    },
  })

  const onSubmit = async (values: TestFormValues) => {
    if (values.webhook_url.trim() === '' && values.email.trim() === '') {
      toast.error(t('Enter a webhook URL or an email address to test'))
      return
    }
    setIsSending(true)
    try {
      const result = await testAlertRule({
        webhook_url: values.webhook_url.trim(),
        webhook_secret: values.webhook_secret,
        email: values.email.trim(),
      })
      if (result.success) {
        toast.success(t('Test notification sent'))
        onOpenChange(false)
        form.reset()
      } else {
        toast.error(result.message || t('Failed to send test notification'))
      }
    } catch {
      toast.error(t('Failed to send test notification'))
    } finally {
      setIsSending(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Send Test Notification')}
      description={t(
        'Verify that a webhook URL and/or email address can receive alert notifications.'
      )}
      contentClassName='sm:max-w-xl'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Close')}
          </Button>
          <Button
            type='submit'
            form='alert-rule-test-form'
            disabled={isSending}
          >
            {isSending ? t('Sending...') : t('Send test')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='alert-rule-test-form'
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='webhook_url'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Webhook URL')}</FormLabel>
                <FormControl>
                  <Input placeholder='https://example.com/hook' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='webhook_secret'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Webhook Secret')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Optional signature secret')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='email'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Email address')}</FormLabel>
                <FormControl>
                  <Input
                    type='email'
                    placeholder={t('Optional notification email')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave fields blank to skip that delivery channel')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
