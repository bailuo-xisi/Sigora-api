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
import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { Gauge } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getCodexQuotas } from '@/features/dashboard/api'
import type {
  CodexQuotaItem,
  CodexQuotaWindow,
} from '@/features/dashboard/types'

function getQuotaWindowLabel(t: TFunction, window: CodexQuotaWindow) {
  if (window.id === 'five-hour') return t('5-hour quota')
  if (window.id === 'weekly') return t('Weekly quota')
  if (window.id === 'monthly') return t('Monthly quota')
  return window.label || window.id || t('Quota')
}

function formatQuotaPercent(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--'
  return `${Math.round(Math.max(0, Math.min(100, value)))}%`
}

function formatResetTime(value?: number | null) {
  if (!value || value <= 0) return ''
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(value * 1000))
}

function getQuotaTone(remaining?: number | null) {
  if (remaining === null || remaining === undefined)
    return 'text-muted-foreground'
  if (remaining <= 15) return 'text-destructive'
  if (remaining <= 35) return 'text-warning'
  return 'text-foreground'
}

function getQuotaBar(remaining?: number | null) {
  if (remaining === null || remaining === undefined)
    return 'bg-muted-foreground'
  if (remaining <= 15) return 'bg-destructive'
  if (remaining <= 35) return 'bg-warning'
  return 'bg-emerald-500'
}

function CodexQuotaWindowRow(props: { window: CodexQuotaWindow }) {
  const { t } = useTranslation()
  const remaining =
    props.window.remaining_percent === null ||
    props.window.remaining_percent === undefined
      ? null
      : Math.max(0, Math.min(100, props.window.remaining_percent))
  const resetTime = formatResetTime(props.window.reset_at)

  return (
    <div className='flex flex-col gap-2'>
      <div className='flex items-center justify-between gap-3 text-xs'>
        <span className='text-foreground truncate font-semibold'>
          {getQuotaWindowLabel(t, props.window)}
        </span>
        <div className='flex shrink-0 items-center gap-1.5'>
          <span
            className={cn(
              'font-mono font-semibold tabular-nums',
              getQuotaTone(remaining)
            )}
          >
            {formatQuotaPercent(remaining)}
          </span>
          {resetTime ? (
            <span className='text-muted-foreground text-[11px] tabular-nums'>
              {resetTime}
            </span>
          ) : null}
        </div>
      </div>
      <div
        className='bg-muted h-2.5 overflow-hidden rounded-full'
        aria-label={getQuotaWindowLabel(t, props.window)}
      >
        <div
          className={cn(
            'h-full rounded-full transition-all',
            getQuotaBar(remaining)
          )}
          style={{ width: `${remaining ?? 0}%` }}
        />
      </div>
      {!resetTime ? (
        <div className='text-muted-foreground truncate text-[11px]'>
          {t('Reset time unavailable')}
        </div>
      ) : null}
    </div>
  )
}

function CodexQuotaItemView(props: {
  item: CodexQuotaItem
  itemIndex: number
}) {
  const { t } = useTranslation()

  return (
    <div className='border-border/70 flex flex-col gap-2 rounded-lg border px-2.5 py-2'>
      {props.item.error ? (
        <div className='text-destructive truncate text-xs'>
          {t('Quota unavailable')}
        </div>
      ) : props.item.windows.length > 0 ? (
        <div className='flex flex-col gap-2'>
          {props.item.windows.map((window) => (
            <CodexQuotaWindowRow
              key={`${props.itemIndex}-${window.id}`}
              window={window}
            />
          ))}
        </div>
      ) : (
        <div className='text-muted-foreground truncate text-xs'>
          {t('No Codex quota data')}
        </div>
      )}
    </div>
  )
}

function CodexQuotaSkeleton() {
  return (
    <div className='flex flex-col gap-2'>
      <Skeleton className='h-4 w-24' />
      <Skeleton className='h-20 w-full' />
    </div>
  )
}

export function CodexQuotaOverview() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['dashboard', 'overview', 'codex-quotas'],
    queryFn: getCodexQuotas,
    retry: false,
    staleTime: 60 * 1000,
    refetchOnWindowFocus: false,
  })

  const items = query.data?.success ? (query.data.data?.items ?? []) : []
  const message =
    query.error instanceof Error
      ? query.error.message
      : query.data?.success === false
        ? query.data.message
        : ''

  return (
    <div className='bg-background/60 flex flex-col gap-2 rounded-lg px-2.5 py-2'>
      <div className='flex items-center justify-between gap-2'>
        <div className='text-muted-foreground flex min-w-0 items-center gap-1.5 text-[11px] leading-none font-medium'>
          <Gauge className='size-3 shrink-0' aria-hidden='true' />
          <span className='truncate'>{t('Codex quota')}</span>
        </div>
        {items.length > 0 ? (
          <Badge variant='outline' className='shrink-0'>
            {items.length}
          </Badge>
        ) : null}
      </div>

      {query.isLoading ? (
        <CodexQuotaSkeleton />
      ) : query.data?.success && query.data.data?.configured === false ? (
        <div className='text-muted-foreground text-xs'>
          {t('Codex quota not configured')}
        </div>
      ) : message ? (
        <div className='text-muted-foreground text-xs'>
          {t('Codex quota unavailable')}
        </div>
      ) : items.length > 0 ? (
        <div className='flex max-h-64 flex-col gap-2 overflow-y-auto pr-1'>
          {items.map((item, index) => (
            <CodexQuotaItemView key={index} item={item} itemIndex={index} />
          ))}
        </div>
      ) : (
        <div className='text-muted-foreground text-xs'>
          {t('No Codex quota data')}
        </div>
      )}
    </div>
  )
}
