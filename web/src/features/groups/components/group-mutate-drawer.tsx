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
import { Box } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Badge } from '@/components/ui/badge'
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
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'

import { updateGroupOption } from '../api'
import {
  useGroupModelsQuery,
  useGroupSettingsQuery,
} from '../hooks/use-group-data'
import {
  formValuesToRateLimit,
  GROUP_FORM_DEFAULT_VALUES,
  groupFormSchema,
  groupToFormValues,
  MAX_RATE_LIMIT,
  type GroupFormValues,
} from '../lib/group-form'
import { applyGroupUpsert, computeOptionUpdates } from '../lib/group-utils'
import type { UserGroup } from '../types'
import { useGroups } from './groups-provider'

type GroupMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: UserGroup
}

export function GroupMutateDrawer(props: GroupMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!props.currentRow
  const { triggerRefresh, refreshTrigger } = useGroups()
  const settingsQuery = useGroupSettingsQuery(refreshTrigger)
  const modelsQuery = useGroupModelsQuery(refreshTrigger)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [duplicateNameError, setDuplicateNameError] = useState<string | null>(
    null
  )

  const form = useForm<GroupFormValues>({
    resolver: zodResolver(groupFormSchema),
    defaultValues: GROUP_FORM_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!props.open) return
    if (isUpdate && props.currentRow) {
      form.reset(groupToFormValues(props.currentRow))
    } else {
      form.reset(GROUP_FORM_DEFAULT_VALUES)
    }
    setDuplicateNameError(null)
  }, [props.open, isUpdate, props.currentRow, form])

  const existingGroupNames = useMemo(() => {
    const settings = settingsQuery.data
    return new Set([
      ...Object.keys(settings.groupRatio),
      ...Object.keys(settings.userUsableGroups),
      ...Object.keys(settings.topupGroupRatio),
      ...Object.keys(settings.rateLimitGroup),
    ])
  }, [settingsQuery.data])

  const watchedSelectable = form.watch('selectable')
  const watchedRateLimitEnabled = form.watch('rateLimitEnabled')
  const modelsForGroup = isUpdate
    ? ((modelsQuery.data ?? {})[props.currentRow?.name ?? ''] ?? [])
    : []

  const onSubmit = async (values: GroupFormValues) => {
    const name = values.name.trim()
    if (!isUpdate && existingGroupNames.has(name)) {
      setDuplicateNameError(t('A group with this name already exists'))
      return
    }
    setDuplicateNameError(null)

    setIsSubmitting(true)
    try {
      const before = settingsQuery.data
      const after = applyGroupUpsert(before, {
        name,
        description: values.description,
        ratio: values.ratio,
        topupRatio: values.topupRatio,
        selectable: values.selectable,
        rateLimit: formValuesToRateLimit(values),
      })
      const updates = computeOptionUpdates(before, after)
      for (const update of updates) {
        const result = await updateGroupOption(update.key, update.value)
        if (!result.success) {
          toast.error(result.message || t('Failed to save group settings'))
          return
        }
      }
      toast.success(
        isUpdate
          ? t('Group updated successfully')
          : t('Group created successfully')
      )
      props.onOpenChange(false)
      triggerRefresh()
    } catch {
      toast.error(t('Failed to save group settings'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Sheet
      open={props.open}
      onOpenChange={(value) => {
        props.onOpenChange(value)
        if (!value) {
          form.reset()
          setDuplicateNameError(null)
        }
      }}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[600px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update') : t('Create')} {t('User Group')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the group pricing, rate limits and model access.')
              : t('Add a new user group by providing its pricing settings.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='group-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            <SideDrawerSection>
              <h3 className='text-sm font-medium'>{t('Basic Information')}</h3>

              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Group name')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('e.g., default, vip, premium')}
                        disabled={isUpdate}
                      />
                    </FormControl>
                    {duplicateNameError ? (
                      <p className='text-destructive text-sm'>
                        {duplicateNameError}
                      </p>
                    ) : (
                      <FormDescription>
                        {isUpdate
                          ? t('Group name cannot be changed when editing.')
                          : t(
                              'Unique identifier used by users, tokens and channels.'
                            )}
                      </FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='ratio'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Base ratio')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={0.1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseFloat(e.target.value) || 0)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Multiplier applied when calls are billed as this group.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='selectable'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='space-y-0.5'>
                      <FormLabel>{t('User selectable')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, users can pick this group when creating API keys.'
                        )}
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

              {watchedSelectable && (
                <FormField
                  control={form.control}
                  name='description'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Description')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Group description')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Shown to users when selecting this group for a token.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </SideDrawerSection>

            <SideDrawerSection>
              <h3 className='text-sm font-medium'>{t('Top-up Pricing')}</h3>
              <FormField
                control={form.control}
                name='topupRatio'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Top-up ratio')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={0.1}
                        {...field}
                        placeholder={t('Not set')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Optional multiplier used when calculating recharge pricing for users in this group. Leave empty for no override.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <SideDrawerSection>
              <div className='flex items-center justify-between'>
                <h3 className='text-sm font-medium'>{t('Rate limit')}</h3>
                <FormField
                  control={form.control}
                  name='rateLimitEnabled'
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          aria-label={t('Enable group rate limit')}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>
              {watchedRateLimitEnabled && (
                <div className='grid gap-4 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='maxRequests'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Max requests (including failures)')}
                        </FormLabel>
                        <FormControl>
                          <div className='flex items-center gap-2'>
                            <Input
                              type='number'
                              min={0}
                              max={MAX_RATE_LIMIT}
                              step={1}
                              {...field}
                              onChange={(e) =>
                                field.onChange(
                                  Number.parseInt(e.target.value) || 0
                                )
                              }
                            />
                            <span className='text-muted-foreground text-sm'>
                              {t('times')}
                            </span>
                          </div>
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Total requests allowed in the window. 0 = unlimited.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='maxSuccess'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Max successful requests')}</FormLabel>
                        <FormControl>
                          <div className='flex items-center gap-2'>
                            <Input
                              type='number'
                              min={1}
                              max={MAX_RATE_LIMIT}
                              step={1}
                              {...field}
                              onChange={(e) =>
                                field.onChange(
                                  Number.parseInt(e.target.value) || 1
                                )
                              }
                            />
                            <span className='text-muted-foreground text-sm'>
                              {t('times')}
                            </span>
                          </div>
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Successful requests are an additional cap; failed requests do not count toward it.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              )}
            </SideDrawerSection>

            <SideDrawerSection>
              <h3 className='text-sm font-medium'>{t('Model access')}</h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Models this group can use, based on the enabled channels that serve the group. Model access is read-only here and is controlled by channel configuration.'
                )}
              </p>
              {modelsForGroup.length === 0 ? (
                <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                  <Box className='h-4 w-4' />
                  {t('No models are enabled for this group.')}
                </div>
              ) : (
                <div className='flex max-h-44 flex-wrap gap-2 overflow-y-auto'>
                  {modelsForGroup.map((modelName) => (
                    <Badge
                      key={modelName}
                      variant='outline'
                      className='font-mono'
                    >
                      {modelName}
                    </Badge>
                  ))}
                </div>
              )}
            </SideDrawerSection>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button form='group-form' type='submit' disabled={isSubmitting}>
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
