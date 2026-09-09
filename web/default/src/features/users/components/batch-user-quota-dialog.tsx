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
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { parseQuotaFromDollars } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog } from '@/components/dialog'
import { batchAdjustUserQuota } from '../api'
import type { QuotaAdjustMode, User } from '../types'

interface BatchUserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  users: User[]
  onSuccess: () => void
}

export function BatchUserQuotaDialog(props: BatchUserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [amount, setAmount] = useState('')
  const quotaMutation = useMutation({ mutationFn: batchAdjustUserQuota })

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const amountValue = Number(amount)
  const quotaValue = Number.isFinite(amountValue)
    ? parseQuotaFromDollars(amountValue)
    : 0
  const validAmount =
    amount.trim() !== '' &&
    Number.isFinite(amountValue) &&
    (mode === 'override' || quotaValue > 0)

  const reset = () => {
    setMode('add')
    setAmount('')
  }

  const handleOpenChange = (open: boolean) => {
    if (!open && quotaMutation.isPending) return
    if (!open) {
      reset()
    }
    props.onOpenChange(open)
  }

  const getModeLabel = (item: QuotaAdjustMode) => {
    switch (item) {
      case 'add':
        return t('Add')
      case 'subtract':
        return t('Subtract')
      case 'override':
        return t('Override')
    }
  }

  const handleConfirm = async () => {
    if (!validAmount || props.users.length === 0) return

    try {
      const result = await quotaMutation.mutateAsync({
        ids: props.users.map((user) => user.id),
        mode,
        value: mode === 'override' ? quotaValue : Math.abs(quotaValue),
      })
      if (result.success) {
        toast.success(t('Quota adjusted successfully'))
        reset()
        props.onSuccess()
      } else {
        toast.error(result.message || t('Failed to adjust quota'))
      }
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to adjust quota')
      )
    }
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={`${t('Adjust Quota')} (${props.users.length})`}
      description={t(
        'Apply the same quota adjustment to {{count}} selected users.',
        { count: props.users.length }
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => handleOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={
              quotaMutation.isPending ||
              !validAmount ||
              props.users.length === 0
            }
          >
            {quotaMutation.isPending ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <Label>{t('Mode')}</Label>
          <div className='flex gap-1'>
            {(['add', 'subtract', 'override'] as const).map((item) => (
              <Button
                key={item}
                type='button'
                variant='outline'
                size='sm'
                className={cn(
                  mode === item &&
                    'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                )}
                onClick={() => {
                  setMode(item)
                  setAmount('')
                }}
              >
                {getModeLabel(item)}
              </Button>
            ))}
          </div>
        </div>

        <div className='space-y-2'>
          <Label>
            {t('Amount')} ({currencyLabel})
          </Label>
          <Input
            type='number'
            step={tokensOnly ? 1 : 0.000001}
            min={mode === 'override' ? undefined : 0}
            placeholder={placeholder}
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') handleConfirm()
            }}
          />
        </div>
      </div>
    </Dialog>
  )
}
