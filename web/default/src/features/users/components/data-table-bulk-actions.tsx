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
import { type Table } from '@tanstack/react-table'
import { WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { type User } from '../types'
import { BatchUserQuotaDialog } from './batch-user-quota-dialog'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false)
  const selectedUsers = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original)

  const handleSuccess = () => {
    setQuotaDialogOpen(false)
    table.resetRowSelection()
    triggerRefresh()
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={() => setQuotaDialogOpen(true)}
                className='size-8'
                aria-label={t('Adjust quota for selected users')}
                title={t('Adjust quota for selected users')}
              />
            }
          >
            <WalletCards />
            <span className='sr-only'>
              {t('Adjust quota for selected users')}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Adjust quota for selected users')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <BatchUserQuotaDialog
        open={quotaDialogOpen}
        onOpenChange={setQuotaDialogOpen}
        users={selectedUsers}
        onSuccess={handleSuccess}
      />
    </>
  )
}
