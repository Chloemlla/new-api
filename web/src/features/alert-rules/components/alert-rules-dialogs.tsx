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
import { AlertRulesDeleteDialog } from './alert-rules-delete-dialog'
import { AlertRulesMutateDialog } from './alert-rules-mutate-dialog'
import { useAlertRules } from './alert-rules-provider'
import { AlertRulesTestDialog } from './alert-rules-test-dialog'

export function AlertRulesDialogs() {
  const { open, setOpen, currentRow } = useAlertRules()

  return (
    <>
      <AlertRulesMutateDialog
        open={open === 'create' || open === 'update'}
        onOpenChange={(v) => {
          if (!v) setOpen(null)
        }}
        currentRow={open === 'update' ? currentRow : null}
      />
      <AlertRulesDeleteDialog
        open={open === 'delete'}
        onOpenChange={(v) => {
          if (!v) setOpen(null)
        }}
        currentRow={currentRow}
      />
      <AlertRulesTestDialog
        open={open === 'test'}
        onOpenChange={(v) => {
          if (!v) setOpen(null)
        }}
      />
    </>
  )
}
