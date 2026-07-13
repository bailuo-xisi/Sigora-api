/*
Copyright (C) 2025 QuantumNous

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

import React, { useEffect, useMemo, useState } from 'react';
import { Button, Spin, Tag } from '@douyinfe/semi-ui';
import { IconExternalOpen, IconRefresh } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const CODEX_RADAR_URL = 'https://codexradar.com/';
const CODEX_RADAR_JSON_URL = 'https://codexradar.com/current.json';
const FIVE_MINUTES = 5 * 60 * 1000;

function formatDateTime(value) {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date);
}

function formatPercent(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--';
  return `${Math.round(value * 100)}%`;
}

function formatNumber(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--';
  return new Intl.NumberFormat().format(value);
}

function formatUsd(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '--';
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
  }).format(value);
}

function formatQuotaTrend(value) {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }
  if (!Array.isArray(value) || value.length === 0) return '--';

  const latestPoint = value[value.length - 1];
  if (!latestPoint || typeof latestPoint !== 'object') {
    return String(value.length);
  }

  const date = latestPoint.date || '--';
  const rate = Number(latestPoint.rate);
  return Number.isFinite(rate)
    ? `${date} · ${formatUsd(rate)}/1%`
    : String(date);
}

function getToneClass(status) {
  const normalized = String(status || '').toLowerCase();
  if (
    normalized.includes('green') ||
    normalized.includes('open') ||
    normalized.includes('allowed')
  ) {
    return 'text-green-600';
  }
  if (normalized.includes('red') || normalized.includes('limit')) {
    return 'text-red-600';
  }
  if (normalized.includes('amber') || normalized.includes('medium')) {
    return 'text-yellow-600';
  }
  return 'text-blue-600';
}

function getScoreToneClass(status) {
  const normalized = String(status || '').toLowerCase();
  if (normalized.includes('green')) return 'text-green-600';
  if (normalized.includes('red')) return 'text-red-600';
  if (normalized.includes('yellow') || normalized.includes('amber')) {
    return 'text-yellow-600';
  }
  return 'text-blue-600';
}

function titleCase(value) {
  if (!value) return '';
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function getModelFamilyLabel(model, label) {
  const source = model || label || '';
  const known = ['sol', 'terra', 'luna', 'opus', 'sonnet', 'haiku'];
  const parts = source.toLowerCase().split(/[\s_-]+/).filter(Boolean);
  const knownPart = parts.find((part) => known.includes(part));
  if (knownPart) return titleCase(knownPart);

  const withoutVersion = source
    .replace(/^gpt[-_ ]?5(?:[._-]?6|[._-]?5)?[-_ ]?/i, '')
    .trim();
  if (withoutVersion) return titleCase(withoutVersion.split(/[\s_-]+/)[0]);
  return source || '--';
}

function getModelIqDisplayLabel(item) {
  const effort = item.reasoning_effort || item.latest?.reasoning_effort;
  const family = getModelFamilyLabel(item.model || item.latest?.model, item.label);
  return [family, effort].filter(Boolean).join(' ') || item.label || item.key;
}

function getModelIqComparisons(modelIq) {
  const primaryLatest =
    modelIq?.latest && typeof modelIq.latest === 'object' ? modelIq.latest : {};
  const primaryRecentDays = Array.isArray(modelIq?.recent_days)
    ? modelIq.recent_days
    : [];
  const primaryLabel =
    [primaryLatest.model, primaryLatest.reasoning_effort]
      .filter(Boolean)
      .join(' ') || '--';
  const items = [
    {
      key: 'primary',
      label: primaryLatest.label || primaryLabel,
      model: primaryLatest.model,
      reasoning_effort: primaryLatest.reasoning_effort,
      latest: primaryLatest,
      recent_days: primaryRecentDays,
    },
  ];

  if (
    modelIq?.comparisons &&
    typeof modelIq.comparisons === 'object' &&
    !Array.isArray(modelIq.comparisons)
  ) {
    Object.entries(modelIq.comparisons).forEach(([key, comparison]) => {
      if (!comparison || typeof comparison !== 'object') return;

      const latest =
        comparison.latest && typeof comparison.latest === 'object'
          ? comparison.latest
          : {};
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
      });
    });
  }

  return items;
}

function MetricCard({ label, value, detail, tone }) {
  return (
    <div className='rounded-2xl border border-semi-color-border bg-semi-color-bg-1 p-4 shadow-sm'>
      <div className='text-xs font-medium text-semi-color-tertiary'>
        {label}
      </div>
      <div className={`mt-2 text-2xl font-semibold ${tone || ''}`}>{value}</div>
      {detail ? (
        <div className='mt-2 text-xs leading-5 text-semi-color-secondary'>
          {detail}
        </div>
      ) : null}
    </div>
  );
}

function SectionCard({ title, action, children }) {
  return (
    <section className='rounded-3xl border border-semi-color-border bg-semi-color-bg-1 p-5 shadow-sm'>
      <div className='mb-4 flex items-start justify-between gap-3'>
        <h2 className='text-lg font-semibold'>{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}

function useCodexRadarSummary() {
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let disposed = false;

    async function load() {
      try {
        const response = await fetch(CODEX_RADAR_JSON_URL, {
          headers: { Accept: 'application/json' },
        });
        if (!response.ok) {
          throw new Error(`Codex Radar request failed: ${response.status}`);
        }
        const json = await response.json();
        if (!disposed) {
          setData(json);
          setError('');
        }
      } catch (err) {
        if (!disposed) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        if (!disposed) setLoading(false);
      }
    }

    load();
    const timer = window.setInterval(load, FIVE_MINUTES);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, []);

  return { data, error, loading };
}

const CodexRadar = () => {
  const { t, i18n } = useTranslation();
  const { data, error, loading } = useCodexRadarSummary();
  const isZh = i18n.language.toLowerCase().startsWith('zh');
  const primaryLatest = data?.model_iq?.latest;
  const [selectedModelKey, setSelectedModelKey] = useState('primary');
  const quotaRadar = data?.model_iq?.quota_radar;
  const quotaCheck = data?.model_iq?.quota_check;
  const tiboPresence = data?.tibo_presence;
  const predictionSummary = isZh
    ? data?.prediction?.summary || data?.prediction?.summary_en
    : data?.prediction?.summary_en || data?.prediction?.summary;
  const tiboEvidence = isZh
    ? tiboPresence?.evidence_summary_zh || tiboPresence?.evidence_summary_en
    : tiboPresence?.evidence_summary_en || tiboPresence?.evidence_summary_zh;
  const tiboLocation = isZh
    ? tiboPresence?.location_label_zh || tiboPresence?.location_label_en
    : tiboPresence?.location_label_en || tiboPresence?.location_label_zh;
  const modelIqItems = useMemo(
    () => getModelIqComparisons(data?.model_iq),
    [data?.model_iq],
  );
  useEffect(() => {
    if (!modelIqItems.some((item) => item.key === selectedModelKey)) {
      setSelectedModelKey(modelIqItems[0]?.key || 'primary');
    }
  }, [modelIqItems, selectedModelKey]);
  const selectedModel = useMemo(
    () =>
      modelIqItems.find((item) => item.key === selectedModelKey) ||
      modelIqItems[0],
    [modelIqItems, selectedModelKey],
  );
  const selectedLatest = useMemo(
    () => selectedModel?.latest || {},
    [selectedModel],
  );
  const selectedRecentDays = useMemo(
    () =>
      Array.isArray(selectedModel?.recent_days)
        ? selectedModel.recent_days.slice(-6).reverse()
        : [],
    [selectedModel],
  );
  const attribution =
    data?.api_access?.requirements?.attribution_text ||
    t('数据来自 Codex 雷达 codexradar.com');

  if (loading) {
    return (
      <div className='classic-page-fill flex min-h-[60vh] items-center justify-center pt-[88px]'>
        <Spin size='large' />
      </div>
    );
  }

  return (
    <div className='classic-page-fill px-4 pb-12 pt-[88px]'>
      <div className='mx-auto flex max-w-6xl flex-col gap-6'>
        <section className='relative overflow-hidden rounded-[2rem] border border-slate-700 bg-slate-950 p-6 text-white shadow-xl md:p-8'>
          <div className='absolute right-8 top-8 hidden text-8xl font-black opacity-10 md:block'>
            RADAR
          </div>
          <div className='relative z-10 max-w-3xl space-y-5'>
            <div className='flex flex-wrap gap-2'>
              <Tag color='blue'>{t('实时公开摘要')}</Tag>
              <Tag color='green'>{t('每 5 分钟自动刷新')}</Tag>
            </div>
            <div className='space-y-3'>
              <p className='text-sm font-medium text-blue-100'>
                {t('Codex 雷达')}
              </p>
              <h1 className='text-3xl font-semibold tracking-tight md:text-5xl'>
                {t('Codex 重置雷达')}
              </h1>
              <p className='max-w-2xl text-base leading-7 text-slate-200'>
                {t(
                  '基于 Codex 雷达公开摘要数据，同步展示 Codex 重置窗口、模型 IQ 和额度信号。',
                )}
              </p>
            </div>
            <div className='flex flex-wrap gap-3'>
              <Button
                theme='solid'
                type='primary'
                icon={<IconExternalOpen />}
                onClick={() => window.open(CODEX_RADAR_URL, '_blank')}
              >
                {t('打开原站')}
              </Button>
              <Button
                theme='borderless'
                icon={<IconExternalOpen />}
                onClick={() => window.open(CODEX_RADAR_JSON_URL, '_blank')}
              >
                {t('查看 JSON')}
              </Button>
            </div>
          </div>
        </section>

        {error ? (
          <div className='rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700'>
            {t('无法加载 Codex 雷达数据，请稍后重试。')}
          </div>
        ) : null}

        <div className='grid gap-4 md:grid-cols-4'>
          <MetricCard
            label={t('重置窗口')}
            value={data?.window?.open ? t('窗口开启') : t('窗口关闭')}
            detail={data?.window?.status || data?.status}
            tone={data?.window?.open ? 'text-green-600' : 'text-slate-500'}
          />
          <MetricCard
            label={t('24 小时概率')}
            value={formatPercent(data?.prediction?.probability_24h)}
            detail={data?.prediction?.level || '--'}
          />
          <MetricCard
            label={t('48 小时概率')}
            value={formatPercent(data?.prediction?.probability_48h)}
            detail={t('更新于 {{time}}', {
              time: formatDateTime(data?.prediction?.updated_at),
            })}
          />
          <MetricCard
            label={t('最新 IQ 分数')}
            value={
              primaryLatest?.score === undefined
                ? '--'
                : String(primaryLatest.score)
            }
            detail={
              primaryLatest
                ? `${primaryLatest.passed ?? '--'}/${primaryLatest.tasks ?? '--'} ${t('通过')}`
                : '--'
            }
            tone={
              primaryLatest?.status === 'green' ? 'text-green-600' : undefined
            }
          />
        </div>

        <div className='grid gap-5 lg:grid-cols-2'>
          <SectionCard
            title={t('重置判断')}
            action={
              <Tag>
                {data?.recommended_action || data?.window?.action || '--'}
              </Tag>
            }
          >
            <div className='space-y-4'>
              <div>
                <h3 className='font-semibold'>
                  {data?.window?.title || t('ChatGPT Work / Codex 重置窗口')}
                </h3>
                <p className='mt-2 text-sm leading-6 text-semi-color-secondary'>
                  {data?.window?.message || t('暂无当前消息。')}
                </p>
              </div>
              <div className='grid gap-3 text-sm md:grid-cols-2'>
                <MetricCard
                  label={t('范围')}
                  value={data?.window?.scope || '--'}
                />
                <MetricCard
                  label={t('关闭时间')}
                  value={formatDateTime(data?.window?.closed_at)}
                />
              </div>
            </div>
          </SectionCard>

          <SectionCard title={t('预测依据')}>
            <p className='text-sm leading-6 text-semi-color-secondary'>
              {predictionSummary || t('暂无预测摘要。')}
            </p>
            <div className='mt-4 grid gap-3 md:grid-cols-3'>
              <MetricCard
                label={t('等级')}
                value={data?.prediction?.level || '--'}
              />
              <MetricCard
                label='24h'
                value={formatPercent(data?.prediction?.probability_24h)}
              />
              <MetricCard
                label='48h'
                value={formatPercent(data?.prediction?.probability_48h)}
              />
            </div>
          </SectionCard>

          <SectionCard
            title={t('模型 IQ')}
            action={
              <Tag className={getToneClass(selectedLatest?.status)}>
                {selectedLatest?.status || '--'}
              </Tag>
            }
          >
            <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
              {modelIqItems.map((item) => {
                const itemLatest = item.latest || {};
                const isSelected = item.key === selectedModelKey;
                const displayLabel = getModelIqDisplayLabel(item);
                return (
                  <button
                    key={item.key}
                    type='button'
                    aria-pressed={isSelected}
                    title={item.label || displayLabel}
                    onClick={() => setSelectedModelKey(item.key)}
                    className={`rounded-2xl border p-4 text-left shadow-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                      isSelected
                        ? 'border-blue-500 bg-blue-50/70 shadow-md ring-4 ring-blue-500/10 dark:bg-blue-950/30'
                        : 'border-semi-color-border bg-semi-color-bg-1 hover:border-blue-300'
                    }`}
                  >
                    <div className='flex min-h-12 items-start justify-between gap-3'>
                      <div className='min-w-0'>
                        <div className='text-base font-semibold leading-5 break-words'>
                          {displayLabel}
                        </div>
                        <div className='mt-1 break-all text-xs text-semi-color-secondary'>
                          {item.model || itemLatest.model || '--'}
                        </div>
                      </div>
                      <div
                        className={`font-mono text-3xl font-semibold ${getScoreToneClass(
                          itemLatest.status,
                        )}`}
                      >
                        {itemLatest.score ?? '--'}
                      </div>
                    </div>
                    <div className='mt-4 grid grid-cols-3 gap-2 text-xs text-semi-color-tertiary'>
                      <div>
                        <div className='font-medium'>
                          {itemLatest.passed ?? '--'}/{itemLatest.tasks ?? '--'}
                        </div>
                        <div>{t('passed')}</div>
                      </div>
                      <div>
                        <div className='font-medium'>
                          {formatUsd(itemLatest.cost_usd)}
                        </div>
                        <div>{formatNumber(itemLatest.total_tokens)}</div>
                      </div>
                      <div className='text-right'>
                        <div className='font-medium'>
                          {itemLatest.wall_time_human || '--'}
                        </div>
                        <div>{itemLatest.status || '--'}</div>
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
            <div className='grid gap-3 md:grid-cols-3'>
              <MetricCard
                label={t('模型')}
                value={selectedLatest?.model || selectedModel?.model || '--'}
              />
              <MetricCard
                label={t('令牌')}
                value={formatNumber(selectedLatest?.total_tokens)}
              />
              <MetricCard
                label={t('成本')}
                value={formatUsd(selectedLatest?.cost_usd)}
              />
            </div>
            <div className='mt-3'>
              <MetricCard
                label={t('Reasoning Effort')}
                value={
                  selectedLatest?.reasoning_effort ||
                  selectedModel?.reasoning_effort ||
                  '--'
                }
              />
            </div>
            <div className='mt-4 space-y-2'>
              {selectedRecentDays.length > 0 ? (
                selectedRecentDays.map((item, index) => (
                  <div
                    key={`${selectedModel?.key || 'primary'}-${item.date || 'unknown'}-${index}`}
                    className='grid grid-cols-[1fr_auto_auto] items-center gap-3 rounded-xl border border-semi-color-border px-3 py-2 text-sm'
                  >
                    <div className='min-w-0'>
                      <div className='truncate font-medium'>
                        {item.date || '--'}
                      </div>
                      <div className='truncate text-xs text-semi-color-tertiary'>
                        {item.wall_time_human || '--'}
                      </div>
                    </div>
                    <Tag className={getToneClass(item.status)}>
                      {item.status || '--'}
                    </Tag>
                    <div className='font-mono font-semibold'>
                      {item.score ?? '--'}
                    </div>
                  </div>
                ))
              ) : (
                <p className='text-sm text-semi-color-secondary'>
                  {t('暂无近期 IQ 数据。')}
                </p>
              )}
            </div>
          </SectionCard>

          <SectionCard title={t('额度雷达')}>
            <div className='grid gap-3 md:grid-cols-2'>
              <MetricCard
                label={t('基准窗口')}
                value={quotaRadar?.basis_window_label || '--'}
              />
              <MetricCard
                label={t('趋势')}
                value={formatQuotaTrend(quotaRadar?.trend)}
              />
              <MetricCard
                label={t('额度检查')}
                value={quotaCheck?.status || '--'}
              />
              <MetricCard
                label={t('重置次数')}
                value={
                  quotaCheck?.rate_limit_reset_credits_available_count ?? '--'
                }
              />
            </div>
            <div className='mt-3 flex items-center gap-2 text-xs text-semi-color-tertiary'>
              <IconRefresh />
              {t('检查于 {{time}}', {
                time: formatDateTime(
                  quotaCheck?.checked_at || quotaRadar?.updated_at,
                ),
              })}
            </div>
          </SectionCard>
        </div>

        {tiboPresence?.should_display ? (
          <SectionCard
            title={t('公开动态信号')}
            action={<Tag>{tiboPresence.confidence || '--'}</Tag>}
          >
            <div className='grid gap-4 lg:grid-cols-[1fr_2fr]'>
              <div className='rounded-2xl border border-semi-color-border p-4'>
                <div className='text-xs text-semi-color-tertiary'>
                  {t('位置上下文')}
                </div>
                <div className='mt-2 text-lg font-semibold'>
                  {tiboLocation || '--'}
                </div>
                <div className='mt-1 text-sm text-semi-color-secondary'>
                  {tiboPresence.timezone || '--'} ·{' '}
                  {formatPercent(tiboPresence.probability)}
                </div>
              </div>
              <div className='rounded-2xl border border-semi-color-border p-4'>
                <p className='text-sm leading-6 text-semi-color-secondary'>
                  {tiboEvidence || '--'}
                </p>
              </div>
            </div>
          </SectionCard>
        ) : null}

        <section className='flex flex-col gap-4 rounded-3xl border border-semi-color-border bg-semi-color-fill-0 p-5 md:flex-row md:items-center md:justify-between'>
          <div className='space-y-1'>
            <div className='font-semibold'>{t('实时数据源')}</div>
            <p className='text-sm leading-6 text-semi-color-secondary'>
              {attribution}.{' '}
              {t(
                '本页面运行时读取公开摘要 JSON，不复制原站图片、二维码或页面资源。',
              )}
            </p>
            <p className='text-xs text-semi-color-tertiary'>
              {t('监测于 {{time}}', {
                time: formatDateTime(data?.monitored_at),
              })}
            </p>
          </div>
          <div className='flex flex-wrap gap-2'>
            <Button
              onClick={() =>
                window.open(
                  data?.links?.rss || `${CODEX_RADAR_URL}feed.xml`,
                  '_blank',
                )
              }
            >
              RSS
            </Button>
            <Button
              icon={<IconExternalOpen />}
              onClick={() =>
                window.open(data?.links?.html || CODEX_RADAR_URL, '_blank')
              }
            >
              {t('原站')}
            </Button>
          </div>
        </section>
      </div>
    </div>
  );
};

export default CodexRadar;
