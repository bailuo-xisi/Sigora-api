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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { Gauge, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getCodexQuotaAllocation,
  getCodexQuotaPool,
  getUpstreamQuotas,
} from '@/features/dashboard/api'
import type {
  CodexQuotaAllocationResult,
  UpstreamQuotaItem,
  UpstreamQuotaProvider,
  UpstreamQuotaWindow,
} from '@/features/dashboard/types'

const CODEX_PENDING_REFETCH_INTERVAL = 30 * 1000
const CODEX_ALLOCATION_REFETCH_INTERVAL = 60 * 1000

function getQuotaWindowLabel(t: TFunction, window: UpstreamQuotaWindow) {
  if (window.id === 'five-hour') return t('5-hour quota')
  if (window.id === 'weekly') return t('Weekly quota')
  if (window.id === 'weekly-opus')
    return t('Weekly quota ({{scope}})', { scope: 'Opus' })
  if (window.id === 'weekly-sonnet')
    return t('Weekly quota ({{scope}})', { scope: 'Sonnet' })
  if (window.id === 'monthly') return t('Monthly quota')
  return window.label || window.id || t('Quota')
}

function formatQuotaPercent(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--'
  return `${Math.round(Math.max(0, Math.min(100, value)))}%`
}

function formatAllocatedUsagePercent(
  usedUnits: number,
  allocatedUnits: number
) {
  if (usedUnits <= 0 || allocatedUnits <= 0) return '0%'
  const percentage = Math.min(100, (usedUnits / allocatedUnits) * 100)
  if (percentage < 0.1) return '<0.1%'
  return `${percentage.toFixed(1)}%`
}

function formatCodexQuotaUnits(value: number) {
  return Math.max(0, Math.round(value)).toLocaleString()
}

function getEstimatedUsedUnits(allocation: CodexQuotaAllocationResult) {
  if (allocation.estimated_used_units !== undefined) {
    return allocation.estimated_used_units
  }
  return allocation.settled_used_units ?? allocation.used_units
}

function getEstimatedPoolLabel(
  t: TFunction,
  windowType: CodexQuotaAllocationResult['window_type']
) {
  if (windowType === 'weekly') return t('Estimated weekly quota pool')
  if (windowType === 'monthly') return t('Estimated monthly quota pool')
  return t('Estimated long-term quota pool')
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

function getQuotaBarColor(remaining?: number | null) {
  if (remaining === null || remaining === undefined) return '#94a3b8'
  if (remaining <= 15) return '#ef4444'
  if (remaining <= 35) return '#f59e0b'
  return '#10b981'
}

function QuotaWindowRow(props: { window: UpstreamQuotaWindow }) {
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
          className='h-full rounded-full transition-all'
          style={{
            width: `${remaining ?? 0}%`,
            backgroundColor: getQuotaBarColor(remaining),
          }}
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

function QuotaItemView(props: {
  item: UpstreamQuotaItem
  emptyMessage: string
}) {
  const { t } = useTranslation()

  return (
    <div className='border-border/70 flex flex-col gap-2 rounded-lg border px-2.5 py-2'>
      {props.item.error ? (
        <div className='text-destructive truncate text-xs'>
          {t('Quota unavailable')}
        </div>
      ) : props.item.windows?.length ? (
        <div className='flex flex-col gap-2'>
          {props.item.windows.map((window) => (
            <QuotaWindowRow key={window.id} window={window} />
          ))}
        </div>
      ) : (
        <div className='text-muted-foreground truncate text-xs'>
          {props.emptyMessage}
        </div>
      )}
    </div>
  )
}

function QuotaSkeleton() {
  return (
    <div className='flex flex-col gap-2'>
      <Skeleton className='h-4 w-24' />
      <Skeleton className='h-20 w-full' />
    </div>
  )
}

export function UpstreamQuotaOverview(props: {
  provider: UpstreamQuotaProvider
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isAdmin = (useAuthStore((state) => state.auth.user?.role) || 0) >= 10
  const isCodex = props.provider === 'codex'
  const showPool = isCodex && isAdmin
  const queryKey = ['dashboard', 'overview', `${props.provider}-quotas`]
  const title = isCodex ? t('Codex quota') : t('Claude quota')
  const emptyMessage = isCodex
    ? t('No Codex quota data')
    : t('No Claude quota data')
  const allocationQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'codex-quota-allocation'],
    queryFn: getCodexQuotaAllocation,
    enabled: isCodex,
    retry: false,
    staleTime: 60 * 1000,
    refetchOnWindowFocus: false,
    refetchInterval: (query) =>
      query.state.data?.success && query.state.data.data?.enabled
        ? CODEX_ALLOCATION_REFETCH_INTERVAL
        : false,
  })
  const allocation =
    isCodex && allocationQuery.data?.success
      ? allocationQuery.data.data
      : undefined
  const estimatedUsedUnits = allocation ? getEstimatedUsedUnits(allocation) : 0
  const pendingSettlementWeight = allocation
    ? allocation.pending_weight + (allocation.awaiting_sync_weight ?? 0)
    : 0
  const hasPendingSettlement = pendingSettlementWeight > 0
  let refetchInterval: number | false = false
  if (allocation?.enabled) refetchInterval = CODEX_ALLOCATION_REFETCH_INTERVAL
  else if (hasPendingSettlement)
    refetchInterval = CODEX_PENDING_REFETCH_INTERVAL

  const query = useQuery({
    queryKey,
    queryFn: () => getUpstreamQuotas(props.provider),
    retry: false,
    staleTime: 60 * 1000,
    refetchOnWindowFocus: false,
    refetchInterval,
  })
  const poolQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'codex-quota-pool'],
    queryFn: getCodexQuotaPool,
    enabled: showPool,
    retry: false,
    staleTime: 60 * 1000,
    refetchOnWindowFocus: false,
    refetchInterval,
  })
  const pool = poolQuery.data?.success ? poolQuery.data.data : undefined
  const refresh = useMutation({
    mutationFn: () => getUpstreamQuotas(props.provider, true),
    onSuccess: async (result) => {
      queryClient.setQueryData(queryKey, result)
      if (isCodex) await allocationQuery.refetch()
      if (showPool) await poolQuery.refetch()
    },
  })
  const isRefreshing =
    refresh.isPending ||
    query.isFetching ||
    (isCodex && allocationQuery.isFetching) ||
    (showPool && poolQuery.isFetching)
  const items = query.data?.success ? (query.data.data?.items ?? []) : []
  const isUnavailable =
    refresh.isError || query.isError || query.data?.success === false

  return (
    <div className='bg-background/60 flex flex-col gap-2 rounded-lg px-2.5 py-2'>
      <div className='flex items-center justify-between gap-2'>
        <div className='text-muted-foreground flex min-w-0 items-center gap-1.5 text-[11px] leading-none font-medium'>
          <Gauge className='size-3 shrink-0' aria-hidden='true' />
          <span className='truncate'>{title}</span>
        </div>
        <div className='flex shrink-0 items-center gap-1'>
          {items.length > 0 ? (
            <Badge variant='outline'>{items.length}</Badge>
          ) : null}
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => refresh.mutate()}
            disabled={isRefreshing}
            className='size-7 p-0'
            aria-label={`${title}: ${t('Refresh')}`}
            title={t('Refresh')}
          >
            <RefreshCw
              className={cn('size-3.5', isRefreshing && 'animate-spin')}
              aria-hidden='true'
            />
          </Button>
        </div>
      </div>

      {allocation?.enabled ? (
        <>
          <div className='border-border/70 grid grid-cols-3 gap-2 rounded-lg border px-2.5 py-2 text-center text-xs'>
            <div>
              <div className='text-muted-foreground'>{t('Assigned')}</div>
              <div className='font-semibold tabular-nums'>
                {(allocation.effective_bps / 100).toFixed(2)}%
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('Estimated usage')}
              </div>
              <div className='font-semibold tabular-nums'>
                {formatAllocatedUsagePercent(
                  estimatedUsedUnits,
                  allocation.allocated_units
                )}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>{t('Status')}</div>
              <div className='font-semibold'>
                {allocation.unattributed_weight > 0
                  ? t('Needs review')
                  : allocation.stale
                    ? t('Stale')
                    : !allocation.pool_available
                      ? t('Codex quota unavailable')
                      : allocation.pending_weight > 0
                        ? t('Pending')
                        : t('Active')}
              </div>
            </div>
          </div>
          <div className='border-border/70 grid grid-cols-3 gap-2 rounded-lg border px-2.5 py-2 text-center text-xs'>
            <div>
              <div className='text-muted-foreground'>
                {t('Allocated units')}
              </div>
              <div className='font-semibold tabular-nums'>
                {formatCodexQuotaUnits(allocation.allocated_units)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>{t('Used units')}</div>
              <div className='font-semibold tabular-nums'>
                {formatCodexQuotaUnits(estimatedUsedUnits)}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground'>
                {t('Remaining quota units')}
              </div>
              <div className='font-semibold tabular-nums'>
                {formatCodexQuotaUnits(allocation.remaining_units)}
              </div>
            </div>
          </div>
          <div className='text-muted-foreground text-center text-[11px]'>
            {t(
              'Each full upstream quota window equals 1,000,000 estimated units; these are not token counts.'
            )}
          </div>
          {allocation.unattributed_weight > 0 ? (
            <div className='text-muted-foreground text-center text-xs tabular-nums'>
              {t('Estimated review weight')}:{' '}
              {allocation.unattributed_weight.toLocaleString()}
            </div>
          ) : null}
          {pendingSettlementWeight > 0 ? (
            <div className='text-muted-foreground text-center text-xs tabular-nums'>
              {t('Estimated pending settlement weight')}:{' '}
              {pendingSettlementWeight.toLocaleString()}
            </div>
          ) : null}
        </>
      ) : null}

      {showPool && pool ? (
        <div className='border-border/70 grid grid-cols-2 gap-2 rounded-lg border px-2.5 py-2 text-center text-xs'>
          <div>
            <div className='text-muted-foreground'>
              {getEstimatedPoolLabel(t, pool.window_type)}
            </div>
            <div className='font-semibold tabular-nums'>
              {formatCodexQuotaUnits(pool.pool_capacity_units)}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground'>{t('Used units')}</div>
            <div className='font-semibold tabular-nums'>
              {formatCodexQuotaUnits(pool.pool_used_units)}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground'>{t('Pool Remaining')}</div>
            <div className='font-semibold tabular-nums'>
              {formatCodexQuotaUnits(pool.pool_remaining_units)}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground'>{t('Total Allocated')}</div>
            <div className='font-semibold tabular-nums'>
              {formatCodexQuotaUnits(pool.allocated_units)}
            </div>
          </div>
          <div className='text-muted-foreground col-span-2 flex justify-center gap-3 text-[11px]'>
            <span>
              {t('Allocated Shares')}: {(pool.allocated_bps / 100).toFixed(2)}%
            </span>
            <span>
              {t('Included Credentials')}: {pool.included_count}
            </span>
          </div>
        </div>
      ) : null}

      {query.isLoading ? (
        <QuotaSkeleton />
      ) : isUnavailable ? (
        <div className='text-muted-foreground text-xs'>
          {isCodex ? t('Codex quota unavailable') : t('Quota unavailable')}
        </div>
      ) : query.data?.success && query.data.data?.configured === false ? (
        <div className='text-muted-foreground text-xs'>
          {isCodex
            ? t('Codex quota not configured')
            : t('Claude quota not configured')}
        </div>
      ) : items.length > 0 ? (
        <div className='flex max-h-64 flex-col gap-2 overflow-y-auto pr-1'>
          {items.map((item, index) => (
            <QuotaItemView
              key={index}
              item={item}
              emptyMessage={emptyMessage}
            />
          ))}
        </div>
      ) : (
        <div className='text-muted-foreground text-xs'>{emptyMessage}</div>
      )}
    </div>
  )
}
