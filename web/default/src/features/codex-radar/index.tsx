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
import { useEffect, useMemo, useState } from 'react'
import type { ComponentType, ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  BarChart3,
  Clock3,
  ExternalLink,
  Gauge,
  Radar,
  RefreshCcw,
  Rss,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PublicLayout } from '@/components/layout'
import { CODEX_RADAR_SUMMARY_URL, getCodexRadarSummary } from './api'
import type {
  CodexRadarModelIqComparison,
  CodexRadarModelIqLatest,
  CodexRadarQuotaRadar,
} from './types'

const ORIGINAL_SITE_URL = 'https://codexradar.com/'
const FIVE_MINUTES = 5 * 60 * 1000

function formatDateTime(value?: string | null) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

function formatPercent(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--'
  return `${Math.round(value * 100)}%`
}

function formatNumber(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--'
  return new Intl.NumberFormat().format(value)
}

function formatUsd(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--'
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
  }).format(value)
}

function formatQuotaTrend(value?: CodexRadarQuotaRadar['trend']) {
  if (typeof value === 'string') return value
  if (!Array.isArray(value) || value.length === 0) return '--'

  const latestPoint = value[value.length - 1]
  const date = latestPoint?.date || '--'
  return typeof latestPoint?.rate === 'number'
    ? `${date} · ${formatUsd(latestPoint.rate)}/1%`
    : date
}

function getToneClass(status?: string) {
  const normalized = (status || '').toLowerCase()
  if (
    normalized.includes('green') ||
    normalized.includes('open') ||
    normalized.includes('allowed')
  ) {
    return 'border-emerald-500/35 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
  }
  if (
    normalized.includes('red') ||
    normalized.includes('limit') ||
    normalized.includes('blocked')
  ) {
    return 'border-destructive/35 bg-destructive/10 text-destructive'
  }
  if (normalized.includes('amber') || normalized.includes('medium')) {
    return 'border-amber-500/35 bg-amber-500/10 text-amber-600 dark:text-amber-300'
  }
  return 'border-blue-500/35 bg-blue-500/10 text-blue-600 dark:text-blue-300'
}

function getScoreToneClass(status?: string) {
  const normalized = (status || '').toLowerCase()
  if (normalized.includes('green'))
    return 'text-emerald-600 dark:text-emerald-300'
  if (normalized.includes('red')) return 'text-destructive'
  if (
    normalized.includes('yellow') ||
    normalized.includes('amber') ||
    normalized.includes('medium')
  ) {
    return 'text-amber-600 dark:text-amber-300'
  }
  return 'text-primary'
}

type ModelIqItem = CodexRadarModelIqComparison & {
  key: string
}

function getModelIqComparisons(modelIq?: {
  latest?: CodexRadarModelIqLatest
  recent_days?: CodexRadarModelIqLatest[]
  comparisons?: Record<string, CodexRadarModelIqComparison>
}) {
  const primaryLatest =
    modelIq?.latest && typeof modelIq.latest === 'object' ? modelIq.latest : {}
  const primaryLabel =
    [primaryLatest.model, primaryLatest.reasoning_effort]
      .filter(Boolean)
      .join(' ') || '--'
  const items: ModelIqItem[] = [
    {
      key: 'primary',
      label: primaryLabel,
      model: primaryLatest.model,
      reasoning_effort: primaryLatest.reasoning_effort,
      latest: primaryLatest,
      recent_days: Array.isArray(modelIq?.recent_days)
        ? modelIq.recent_days
        : [],
    },
  ]

  if (
    modelIq?.comparisons &&
    typeof modelIq.comparisons === 'object' &&
    !Array.isArray(modelIq.comparisons)
  ) {
    Object.entries(modelIq.comparisons).forEach(([key, comparison]) => {
      if (!comparison || typeof comparison !== 'object') return

      const latest =
        comparison.latest && typeof comparison.latest === 'object'
          ? comparison.latest
          : {}
      items.push({
        key,
        label: comparison.label || latest.label || key,
        model: comparison.model || latest.model,
        reasoning_effort:
          comparison.reasoning_effort || latest.reasoning_effort,
        latest,
        recent_days: Array.isArray(comparison.recent_days)
          ? comparison.recent_days
          : [],
      })
    })
  }

  return items
}

function titleCase(value?: string) {
  if (!value) return ''
  return value.charAt(0).toUpperCase() + value.slice(1)
}

function getModelFamilyLabel(model?: string, label?: string) {
  const source = model || label || ''
  const known = ['sol', 'terra', 'luna', 'opus', 'sonnet', 'haiku']
  const parts = source
    .toLowerCase()
    .split(/[\s_-]+/)
    .filter(Boolean)
  const knownPart = parts.find((part) => known.includes(part))
  if (knownPart) return titleCase(knownPart)

  const withoutVersion = source
    .replace(/^gpt[-_ ]?5(?:[._-]?6|[._-]?5)?[-_ ]?/i, '')
    .trim()
  if (withoutVersion) return titleCase(withoutVersion.split(/[\s_-]+/)[0])
  return source || '--'
}

function getModelIqDisplayLabel(item: ModelIqItem) {
  const effort = item.reasoning_effort || item.latest?.reasoning_effort
  const family = getModelFamilyLabel(
    item.model || item.latest?.model,
    item.label
  )
  return [family, effort].filter(Boolean).join(' ') || item.label || item.key
}

function MetricCard(props: {
  label: string
  value: string
  detail?: string
  tone?: string
}) {
  return (
    <div className='bg-background/70 rounded-2xl border p-4 shadow-sm backdrop-blur'>
      <div className='text-muted-foreground text-xs font-medium'>
        {props.label}
      </div>
      <div
        className={cn('mt-2 text-2xl font-semibold tracking-tight', props.tone)}
      >
        {props.value}
      </div>
      {props.detail ? (
        <div className='text-muted-foreground mt-2 text-xs leading-5'>
          {props.detail}
        </div>
      ) : null}
    </div>
  )
}

function SectionCard(props: {
  icon: ComponentType<{ className?: string }>
  title: string
  children: ReactNode
  action?: ReactNode
}) {
  const Icon = props.icon
  return (
    <section className='bg-background/75 rounded-3xl border p-5 shadow-sm backdrop-blur'>
      <div className='mb-4 flex items-start justify-between gap-3'>
        <div className='flex items-center gap-3'>
          <div className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-2xl'>
            <Icon className='size-4' />
          </div>
          <h2 className='text-lg font-semibold'>{props.title}</h2>
        </div>
        {props.action}
      </div>
      {props.children}
    </section>
  )
}

function LoadingView() {
  return (
    <PublicLayout>
      <div className='mx-auto flex max-w-6xl flex-col gap-6 px-4 pt-8 pb-16'>
        <Skeleton className='h-52 w-full rounded-3xl' />
        <div className='grid gap-4 md:grid-cols-4'>
          <Skeleton className='h-28 rounded-2xl' />
          <Skeleton className='h-28 rounded-2xl' />
          <Skeleton className='h-28 rounded-2xl' />
          <Skeleton className='h-28 rounded-2xl' />
        </div>
        <div className='grid gap-5 lg:grid-cols-2'>
          <Skeleton className='h-64 rounded-3xl' />
          <Skeleton className='h-64 rounded-3xl' />
        </div>
      </div>
    </PublicLayout>
  )
}

function RecentIqRow(props: { item: CodexRadarModelIqLatest }) {
  return (
    <div className='grid grid-cols-[1fr_auto_auto] items-center gap-3 rounded-xl border px-3 py-2 text-sm'>
      <div className='min-w-0'>
        <div className='truncate font-medium'>{props.item.date || '--'}</div>
        <div className='text-muted-foreground truncate text-xs'>
          {props.item.wall_time_human || '--'}
        </div>
      </div>
      <Badge variant='outline' className={getToneClass(props.item.status)}>
        {props.item.status || '--'}
      </Badge>
      <div className='font-mono text-sm font-semibold tabular-nums'>
        {props.item.score ?? '--'}
      </div>
    </div>
  )
}

function ModelIqChoiceCard(props: {
  item: ModelIqItem
  selected: boolean
  onSelect: () => void
  passedLabel: string
}) {
  const latest = props.item.latest || {}
  const scoreTone = getScoreToneClass(latest.status)
  const displayLabel = getModelIqDisplayLabel(props.item)

  return (
    <button
      type='button'
      aria-pressed={props.selected}
      title={props.item.label || displayLabel}
      onClick={props.onSelect}
      className={cn(
        'bg-background/80 hover:border-primary/40 focus-visible:ring-ring rounded-2xl border p-4 text-left shadow-sm transition focus-visible:ring-2 focus-visible:outline-none',
        props.selected
          ? 'border-primary/60 ring-primary/15 shadow-md ring-4'
          : 'border-border'
      )}
    >
      <div className='flex min-h-12 items-start justify-between gap-3'>
        <div className='min-w-0'>
          <div className='text-base leading-5 font-semibold break-words'>
            {displayLabel}
          </div>
          <div className='text-muted-foreground mt-1 text-xs break-all'>
            {props.item.model || latest.model || '--'}
          </div>
        </div>
        <div className={cn('font-mono text-3xl font-semibold', scoreTone)}>
          {latest.score ?? '--'}
        </div>
      </div>

      <div className='text-muted-foreground mt-4 grid grid-cols-3 gap-2 text-xs'>
        <div>
          <div className='font-medium'>
            {latest.passed ?? '--'}/{latest.tasks ?? '--'}
          </div>
          <div>{props.passedLabel}</div>
        </div>
        <div>
          <div className='font-medium'>{formatUsd(latest.cost_usd)}</div>
          <div>{formatNumber(latest.total_tokens)}</div>
        </div>
        <div className='text-right'>
          <div className='font-medium'>{latest.wall_time_human || '--'}</div>
          <div>{latest.status || '--'}</div>
        </div>
      </div>
    </button>
  )
}

export function CodexRadarPage() {
  const { t, i18n } = useTranslation()
  const query = useQuery({
    queryKey: ['codex-radar', 'public-summary'],
    queryFn: getCodexRadarSummary,
    staleTime: FIVE_MINUTES,
    refetchInterval: FIVE_MINUTES,
    refetchOnWindowFocus: false,
    retry: 1,
  })

  const data = query.data
  const isZh = i18n.language.toLowerCase().startsWith('zh')
  const latest = data?.model_iq?.latest
  const [selectedModelKey, setSelectedModelKey] = useState('primary')
  const quotaRadar = data?.model_iq?.quota_radar
  const quotaCheck = data?.model_iq?.quota_check
  const tiboPresence = data?.tibo_presence
  const predictionSummary = isZh
    ? data?.prediction?.summary || data?.prediction?.summary_en
    : data?.prediction?.summary_en || data?.prediction?.summary
  const tiboEvidence = isZh
    ? tiboPresence?.evidence_summary_zh || tiboPresence?.evidence_summary_en
    : tiboPresence?.evidence_summary_en || tiboPresence?.evidence_summary_zh
  const tiboLocation = isZh
    ? tiboPresence?.location_label_zh || tiboPresence?.location_label_en
    : tiboPresence?.location_label_en || tiboPresence?.location_label_zh
  const attribution =
    data?.api_access?.requirements?.attribution_text ||
    t('Data from Codex Radar codexradar.com')
  const statusLabel = data?.window?.open ? t('Window open') : t('Window closed')
  const recommendedAction = data?.recommended_action || data?.window?.action
  const modelIqItems = useMemo(
    () => getModelIqComparisons(data?.model_iq),
    [data?.model_iq]
  )
  useEffect(() => {
    if (!modelIqItems.some((item) => item.key === selectedModelKey)) {
      setSelectedModelKey(modelIqItems[0]?.key || 'primary')
    }
  }, [modelIqItems, selectedModelKey])
  const selectedModel = useMemo(
    () =>
      modelIqItems.find((item) => item.key === selectedModelKey) ||
      modelIqItems[0],
    [modelIqItems, selectedModelKey]
  )
  const selectedLatest: CodexRadarModelIqLatest = selectedModel?.latest || {}
  const selectedRecentDays = useMemo(
    () =>
      Array.isArray(selectedModel?.recent_days)
        ? selectedModel.recent_days.slice(-6).reverse()
        : [],
    [selectedModel]
  )

  if (query.isLoading) return <LoadingView />

  return (
    <PublicLayout>
      <div className='mx-auto flex max-w-6xl flex-col gap-6 px-4 pt-8 pb-16'>
        <section className='relative overflow-hidden rounded-[2rem] border bg-[radial-gradient(circle_at_20%_0%,rgba(59,130,246,0.22),transparent_35%),linear-gradient(135deg,rgba(15,23,42,0.96),rgba(30,41,59,0.92))] p-6 text-white shadow-xl md:p-8'>
          <div className='absolute top-8 right-8 hidden opacity-20 md:block'>
            <Radar className='size-36' />
          </div>
          <div className='relative z-10 max-w-3xl space-y-5'>
            <div className='flex flex-wrap items-center gap-2'>
              <Badge className='bg-white/15 text-white hover:bg-white/20'>
                <Activity className='size-3' />
                {t('Live public summary')}
              </Badge>
              <Badge className='border-white/20 bg-white/10 text-white'>
                {t('Auto refresh every 5 minutes')}
              </Badge>
            </div>
            <div className='space-y-3'>
              <p className='text-sm font-medium text-blue-100'>
                {t('Codex Radar')}
              </p>
              <h1 className='text-3xl font-semibold tracking-tight md:text-5xl'>
                {t('Codex reset radar')}
              </h1>
              <p className='max-w-2xl text-base leading-7 text-slate-200'>
                {t(
                  'A live mirror of Codex reset window, model IQ, and quota signals based on Codex Radar public summary data.'
                )}
              </p>
            </div>
            <div className='flex flex-wrap gap-3'>
              <Button
                className='bg-white text-slate-950 hover:bg-slate-100'
                render={
                  <a
                    href={ORIGINAL_SITE_URL}
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                }
              >
                {t('Open original site')}
                <ExternalLink className='size-4' />
              </Button>
              <Button
                variant='outline'
                className='border-white/25 bg-white/10 text-white hover:bg-white/15 hover:text-white'
                render={
                  <a
                    href={CODEX_RADAR_SUMMARY_URL}
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                }
              >
                {t('View JSON')}
                <ExternalLink className='size-4' />
              </Button>
            </div>
          </div>
        </section>

        {query.error ? (
          <div className='border-destructive/30 bg-destructive/10 text-destructive rounded-2xl border p-4 text-sm'>
            {t('Unable to load Codex Radar data. Please try again later.')}
          </div>
        ) : null}

        <div className='grid gap-4 md:grid-cols-4'>
          <MetricCard
            label={t('Reset window')}
            value={statusLabel}
            detail={data?.window?.status || data?.status}
            tone={data?.window?.open ? 'text-emerald-500' : 'text-slate-500'}
          />
          <MetricCard
            label={t('24-hour probability')}
            value={formatPercent(data?.prediction?.probability_24h)}
            detail={data?.prediction?.level || '--'}
          />
          <MetricCard
            label={t('48-hour probability')}
            value={formatPercent(data?.prediction?.probability_48h)}
            detail={t('Updated {{time}}', {
              time: formatDateTime(data?.prediction?.updated_at),
            })}
          />
          <MetricCard
            label={t('Latest IQ score')}
            value={latest?.score === undefined ? '--' : String(latest.score)}
            detail={
              latest
                ? `${latest.passed ?? '--'}/${latest.tasks ?? '--'} ${t('passed')}`
                : '--'
            }
            tone={latest?.status === 'green' ? 'text-emerald-500' : undefined}
          />
        </div>

        <div className='grid gap-5 lg:grid-cols-2'>
          <SectionCard
            icon={ShieldCheck}
            title={t('Reset judgement')}
            action={
              <Badge variant='outline' className={getToneClass(data?.status)}>
                {recommendedAction || '--'}
              </Badge>
            }
          >
            <div className='space-y-4'>
              <div>
                <h3 className='font-semibold'>
                  {data?.window?.title ||
                    t('ChatGPT Work / Codex reset window')}
                </h3>
                <p className='text-muted-foreground mt-2 text-sm leading-6'>
                  {data?.window?.message || t('No current message available.')}
                </p>
              </div>
              <div className='grid gap-3 text-sm md:grid-cols-2'>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Scope')}
                  </div>
                  <div className='mt-1 font-medium'>
                    {data?.window?.scope || '--'}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Closed at')}
                  </div>
                  <div className='mt-1 font-medium'>
                    {formatDateTime(data?.window?.closed_at)}
                  </div>
                </div>
              </div>
              {data?.window?.source_url ? (
                <Button
                  variant='outline'
                  size='sm'
                  render={
                    <a
                      href={data.window.source_url}
                      target='_blank'
                      rel='noopener noreferrer'
                    />
                  }
                >
                  {t('Source')}
                  <ExternalLink className='size-3.5' />
                </Button>
              ) : null}
            </div>
          </SectionCard>

          <SectionCard icon={Sparkles} title={t('Prediction rationale')}>
            <div className='space-y-4'>
              <p className='text-muted-foreground text-sm leading-6'>
                {predictionSummary || t('No prediction summary available.')}
              </p>
              <div className='grid gap-3 md:grid-cols-3'>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Level')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {data?.prediction?.level || '--'}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>24h</div>
                  <div className='mt-1 font-semibold'>
                    {formatPercent(data?.prediction?.probability_24h)}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>48h</div>
                  <div className='mt-1 font-semibold'>
                    {formatPercent(data?.prediction?.probability_48h)}
                  </div>
                </div>
              </div>
            </div>
          </SectionCard>

          <SectionCard
            icon={Gauge}
            title={t('Model IQ')}
            action={
              <Badge
                variant='outline'
                className={getToneClass(selectedLatest?.status)}
              >
                {selectedLatest?.status || '--'}
              </Badge>
            }
          >
            <div className='space-y-4'>
              <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
                {modelIqItems.map((item) => (
                  <ModelIqChoiceCard
                    key={item.key}
                    item={item}
                    selected={item.key === selectedModelKey}
                    onSelect={() => setSelectedModelKey(item.key)}
                    passedLabel={t('passed')}
                  />
                ))}
              </div>
              <div className='grid gap-3 md:grid-cols-3'>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Model')}
                  </div>
                  <div className='mt-1 font-semibold break-all'>
                    {selectedLatest?.model || selectedModel?.model || '--'}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Tokens')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {formatNumber(selectedLatest?.total_tokens)}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Cost')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {formatUsd(selectedLatest?.cost_usd)}
                  </div>
                </div>
              </div>
              <div className='rounded-xl border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Reasoning Effort')}
                </div>
                <div className='mt-1 font-semibold'>
                  {selectedLatest?.reasoning_effort ||
                    selectedModel?.reasoning_effort ||
                    '--'}
                </div>
              </div>
              <div className='space-y-2'>
                {selectedRecentDays.length > 0 ? (
                  selectedRecentDays.map((item, index) => (
                    <RecentIqRow
                      key={`${selectedModel?.key || 'primary'}-${item.date || 'unknown'}-${index}`}
                      item={item}
                    />
                  ))
                ) : (
                  <p className='text-muted-foreground text-sm'>
                    {t('No recent IQ data available.')}
                  </p>
                )}
              </div>
            </div>
          </SectionCard>

          <SectionCard icon={BarChart3} title={t('Quota radar')}>
            <div className='space-y-4'>
              <div className='grid gap-3 md:grid-cols-2'>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Basis window')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {quotaRadar?.basis_window_label || '--'}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Trend')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {formatQuotaTrend(quotaRadar?.trend)}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Quota check')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {quotaCheck?.status || '--'}
                  </div>
                </div>
                <div className='rounded-xl border p-3'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Reset credits')}
                  </div>
                  <div className='mt-1 font-semibold'>
                    {quotaCheck?.rate_limit_reset_credits_available_count ??
                      '--'}
                  </div>
                </div>
              </div>
              <div className='text-muted-foreground flex items-center gap-2 text-xs'>
                <Clock3 className='size-3.5' />
                {t('Checked {{time}}', {
                  time: formatDateTime(
                    quotaCheck?.checked_at || quotaRadar?.updated_at
                  ),
                })}
              </div>
            </div>
          </SectionCard>
        </div>

        {tiboPresence?.should_display ? (
          <SectionCard
            icon={Radar}
            title={t('Public presence signal')}
            action={
              <Badge
                variant='outline'
                className={getToneClass(tiboPresence.confidence)}
              >
                {tiboPresence.confidence || '--'}
              </Badge>
            }
          >
            <div className='grid gap-4 lg:grid-cols-[1fr_2fr]'>
              <div className='rounded-2xl border p-4'>
                <div className='text-muted-foreground text-xs'>
                  {t('Location context')}
                </div>
                <div className='mt-2 text-lg font-semibold'>
                  {tiboLocation || '--'}
                </div>
                <div className='text-muted-foreground mt-1 text-sm'>
                  {tiboPresence.timezone || '--'} ·{' '}
                  {formatPercent(tiboPresence.probability)}
                </div>
              </div>
              <div className='rounded-2xl border p-4'>
                <p className='text-muted-foreground text-sm leading-6'>
                  {tiboEvidence || '--'}
                </p>
                <p className='text-muted-foreground mt-3 text-xs leading-5'>
                  {isZh
                    ? tiboPresence.safety_note_zh
                    : tiboPresence.safety_note_en}
                </p>
              </div>
            </div>
          </SectionCard>
        ) : null}

        <section className='bg-muted/35 flex flex-col gap-4 rounded-3xl border p-5 md:flex-row md:items-center md:justify-between'>
          <div className='space-y-1'>
            <div className='flex items-center gap-2 font-semibold'>
              <RefreshCcw className='size-4' />
              {t('Live data source')}
            </div>
            <p className='text-muted-foreground text-sm leading-6'>
              {attribution}.{' '}
              {t(
                'This page reads the public summary JSON at runtime instead of copying the original site assets.'
              )}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Monitored at {{time}}', {
                time: formatDateTime(data?.monitored_at),
              })}
            </p>
          </div>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              size='sm'
              render={
                <a
                  href={data?.links?.rss || `${ORIGINAL_SITE_URL}feed.xml`}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              <Rss className='size-3.5' />
              RSS
            </Button>
            <Button
              variant='outline'
              size='sm'
              render={
                <a
                  href={data?.links?.html || ORIGINAL_SITE_URL}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              {t('Original')}
              <ExternalLink className='size-3.5' />
            </Button>
          </div>
        </section>
      </div>
    </PublicLayout>
  )
}
