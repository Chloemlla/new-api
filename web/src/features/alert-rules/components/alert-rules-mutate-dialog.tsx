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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { createAlertRule, updateAlertRule } from '../api'
import {
  SCOPE_OPTIONS,
  SUCCESS_MESSAGES,
  TRIGGER_TYPE_OPTIONS,
} from '../constants'
import {
  ALERT_RULE_FORM_DEFAULT_VALUES,
  getAlertRuleFormSchema,
  transformFormValuesToPayload,
  transformRuleToFormValues,
  type AlertRuleFormValues,
} from '../lib/alert-rule-form'
import type { AlertRule } from '../types'
import { useAlertRules } from './alert-rules-provider'

const FORM_ID = 'alert-rule-form'

type AlertRulesMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow: AlertRule | null
}

export function AlertRulesMutateDialog({
  open,
  onOpenChange,
  currentRow,
}: AlertRulesMutateDialogProps) {
  const { t } = useTranslation()
  const isUpdate = currentRow !== null
  const { triggerRefresh } = useAlertRules()

  const form = useForm<AlertRuleFormValues>({
    resolver: zodResolver(getAlertRuleFormSchema(t)),
    defaultValues: ALERT_RULE_FORM_DEFAULT_VALUES,
  })

  const triggerType = form.watch('trigger_type')
  const scope = form.watch('scope')

  useEffect(() => {
    if (open) {
      form.reset(
        currentRow
          ? transformRuleToFormValues(currentRow)
          : ALERT_RULE_FORM_DEFAULT_VALUES
      )
    }
  }, [open, currentRow, form])

  const onSubmit = async (values: AlertRuleFormValues) => {
    const payload = transformFormValuesToPayload(values)
    try {
      const result =
        isUpdate && currentRow
          ? await updateAlertRule({ ...payload, id: currentRow.id })
          : await createAlertRule(payload)
      if (result.success) {
        toast.success(
          t(
            isUpdate
              ? SUCCESS_MESSAGES.ALERT_RULE_UPDATED
              : SUCCESS_MESSAGES.ALERT_RULE_CREATED
          )
        )
        onOpenChange(false)
        triggerRefresh()
      }
    } catch {
      toast.error(
        t(
          isUpdate
            ? 'Failed to update alert rule'
            : 'Failed to create alert rule'
        )
      )
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isUpdate ? t('Update Alert Rule') : t('Create Alert Rule')}
      description={t(
        'Configure a trigger and where the alert notification is delivered.'
      )}
      contentClassName='sm:max-w-2xl'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={FORM_ID}>
            {isUpdate ? t('Save changes') : t('Create')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Enter a name for this alert rule')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between rounded-md border p-3'>
                <div>
                  <FormLabel>{t('Enabled')}</FormLabel>
                  <FormDescription>
                    {t('Whether this alert rule is active')}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='trigger_type'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Trigger')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t('Select a trigger type')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectGroup>
                        {TRIGGER_TYPE_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {t(option.labelKey)}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='threshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {triggerType === 'channel_balance'
                      ? t('Balance threshold (USD)')
                      : t('Failure rate threshold (%)')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min='0'
                      step={triggerType === 'channel_balance' ? 0.01 : 1}
                      placeholder={
                        triggerType === 'channel_balance' ? '10' : '50'
                      }
                      onChange={(e) =>
                        field.onChange(
                          e.target.value === ''
                            ? 0
                            : Number.parseFloat(e.target.value)
                        )
                      }
                      value={field.value}
                    />
                  </FormControl>
                  <FormDescription>
                    {triggerType === 'channel_balance'
                      ? t(
                          'Alert when a channel balance drops below this amount'
                        )
                      : t(
                          'Alert when a channel failure rate exceeds this percent'
                        )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {triggerType === 'channel_failure_rate' && (
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='window_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Window (minutes)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min='1'
                        onChange={(e) =>
                          field.onChange(
                            e.target.value === ''
                              ? 1
                              : Number.parseInt(e.target.value, 10)
                          )
                        }
                        value={field.value}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Evaluate requests from the last N minutes')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='min_sample_count'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Min sample count')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min='0'
                        onChange={(e) =>
                          field.onChange(
                            e.target.value === ''
                              ? 0
                              : Number.parseInt(e.target.value, 10)
                          )
                        }
                        value={field.value}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Only alert when at least this many requests were made'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}

          <FormField
            control={form.control}
            name='scope'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Channel scope')}</FormLabel>
                <Select value={field.value} onValueChange={field.onChange}>
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue placeholder={t('Select a channel scope')} />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectGroup>
                      {SCOPE_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {t(option.labelKey)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          {scope === 'tag' && (
            <FormField
              control={form.control}
              name='channel_tag'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Channel tag')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('e.g. partner')} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Only channels with this tag are watched')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          {scope === 'ids' && (
            <FormField
              control={form.control}
              name='channel_ids'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Channel IDs')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('e.g. 1, 3, 7')} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Comma-separated list of channel IDs to watch')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <div className='grid gap-4 sm:grid-cols-2'>
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
          </div>

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
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='cooldown_minutes'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Cooldown (minutes)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='0'
                    onChange={(e) =>
                      field.onChange(
                        e.target.value === ''
                          ? 0
                          : Number.parseInt(e.target.value, 10)
                      )
                    }
                    value={field.value}
                  />
                </FormControl>
                <FormDescription>
                  {t('Minimum wait between notifications for the same rule')}
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
