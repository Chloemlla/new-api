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

import { SectionPageLayout } from '@/components/layout'

import { GroupDeleteDialog } from './components/group-delete-dialog'
import { GroupMutateDrawer } from './components/group-mutate-drawer'
import { GroupsPrimaryButtons } from './components/groups-primary-buttons'
import { GroupsProvider, useGroups } from './components/groups-provider'
import { GroupsTable } from './components/groups-table'

function GroupsContent() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useGroups()

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('User Groups')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <GroupsPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <GroupsTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <GroupMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? currentRow || undefined : undefined}
      />
      <GroupDeleteDialog />
    </>
  )
}

export function UserGroups() {
  return (
    <GroupsProvider>
      <GroupsContent />
    </GroupsProvider>
  )
}
