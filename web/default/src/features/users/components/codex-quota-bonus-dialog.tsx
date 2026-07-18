import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog } from '@/components/dialog'
import { adjustUserCodexQuotaBonus } from '../api'
import type { QuotaAdjustMode } from '../types'

type CodexQuotaBonusDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentBonusBps: number
  onSuccess: () => void
}

export function CodexQuotaBonusDialog(props: CodexQuotaBonusDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [value, setValue] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async () => {
    const percent = Number(value)
    if (!Number.isFinite(percent) || percent < 0 || percent > 100) return
    setLoading(true)
    try {
      const result = await adjustUserCodexQuotaBonus(props.userId, {
        mode,
        value_bps: Math.round(percent * 100),
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to adjust Codex extra share'))
        return
      }
      toast.success(t('Codex extra share updated'))
      setValue('')
      props.onOpenChange(false)
      props.onSuccess()
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to adjust Codex extra share')
      )
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Adjust Codex Extra Share')}
      description={t(
        'Extra share is persistent, and site-wide allocated shares may exceed 100%.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={submit} disabled={loading}>
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='text-muted-foreground text-sm'>
        {t('Current extra share: {{percent}}%', {
          percent: (props.currentBonusBps / 100).toFixed(2),
        })}
      </div>
      <div className='flex gap-2'>
        {(['add', 'subtract', 'override'] as const).map((item) => (
          <Button
            key={item}
            type='button'
            size='sm'
            variant={mode === item ? 'default' : 'outline'}
            onClick={() => setMode(item)}
          >
            {item === 'add'
              ? t('Add')
              : item === 'subtract'
                ? t('Subtract')
                : t('Override')}
          </Button>
        ))}
      </div>
      <div className='space-y-2'>
        <Label>{t('Share Percentage')}</Label>
        <Input
          type='number'
          min={0}
          max={100}
          step={0.01}
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      </div>
    </Dialog>
  )
}
