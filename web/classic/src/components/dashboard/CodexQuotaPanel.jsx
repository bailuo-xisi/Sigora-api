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

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Card, Skeleton } from '@douyinfe/semi-ui';
import { Gauge, RefreshCw } from 'lucide-react';
import { API, isAdmin } from '../../helpers';
import ScrollableContainer from '../common/ui/ScrollableContainer';

const CODEX_PENDING_REFRESH_INTERVAL = 30 * 1000;

const clampPercent = (value) => {
  if (value === null || value === undefined || Number.isNaN(Number(value))) {
    return null;
  }
  return Math.max(0, Math.min(100, Number(value)));
};

const formatPercent = (value) => {
  const percent = clampPercent(value);
  if (percent === null) return '--';
  return `${Math.round(percent)}%`;
};

const formatAllocatedUsagePercent = (usedUnits, allocatedUnits) => {
  if (usedUnits <= 0 || allocatedUnits <= 0) return '0%';
  const percentage = Math.min(100, (usedUnits / allocatedUnits) * 100);
  if (percentage < 0.1) return '<0.1%';
  return `${percentage.toFixed(1)}%`;
};

const formatResetTime = (value) => {
  if (!value || value <= 0) return '';
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(value * 1000));
};

const getWindowLabel = (t, window) => {
  if (window.id === 'five-hour') return t('5-hour quota');
  if (window.id === 'weekly') return t('Weekly quota');
  if (window.id === 'monthly') return t('Monthly quota');
  return window.label || window.id || t('Quota');
};

const getQuotaToneClass = (remaining) => {
  if (remaining === null || remaining === undefined) return 'text-gray-500';
  if (remaining <= 15) return 'text-red-500';
  if (remaining <= 35) return 'text-amber-500';
  return 'text-gray-900';
};

const getQuotaBarColor = (remaining) => {
  if (remaining === null || remaining === undefined) return '#cbd5e1';
  if (remaining <= 15) return '#ef4444';
  if (remaining <= 35) return '#f59e0b';
  return '#10b981';
};

const CodexQuotaWindowRow = ({ quotaWindow, t }) => {
  const remaining = clampPercent(quotaWindow.remaining_percent);
  const resetTime = formatResetTime(quotaWindow.reset_at);

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-3 text-sm'>
        <span className='min-w-0 truncate font-semibold text-gray-900'>
          {getWindowLabel(t, quotaWindow)}
        </span>
        <div className='flex shrink-0 items-center gap-2'>
          <span
            className={`font-mono font-semibold tabular-nums ${getQuotaToneClass(
              remaining,
            )}`}
          >
            {formatPercent(remaining)}
          </span>
          {resetTime && (
            <span className='text-xs tabular-nums text-gray-400'>
              {resetTime}
            </span>
          )}
        </div>
      </div>
      <div
        className='h-3 overflow-hidden rounded-full'
        style={{ backgroundColor: '#eef2f7' }}
      >
        <div
          className='h-full rounded-full transition-all'
          style={{
            width: `${remaining ?? 0}%`,
            backgroundColor: getQuotaBarColor(remaining),
          }}
        />
      </div>
      {!resetTime && (
        <div className='truncate text-[11px] text-gray-400'>
          {t('Reset time unavailable')}
        </div>
      )}
    </div>
  );
};

const CodexQuotaItem = ({ item, itemIndex, t }) => {
  return (
    <div className='rounded-xl border border-gray-100 bg-white p-3 shadow-sm'>
      {item.error ? (
        <div className='truncate text-xs text-red-500' title={item.error}>
          {t('Quota unavailable')}
          {item.error_status ? ` (${item.error_status})` : ''}
        </div>
      ) : item.windows && item.windows.length > 0 ? (
        <div className='space-y-3'>
          {item.windows.map((quotaWindow) => (
            <CodexQuotaWindowRow
              key={`${itemIndex}-${quotaWindow.id}`}
              quotaWindow={quotaWindow}
              t={t}
            />
          ))}
        </div>
      ) : (
        <div className='text-xs text-gray-500'>{t('No Codex quota data')}</div>
      )}
    </div>
  );
};

const CodexQuotaPanel = ({ CARD_PROPS, FLEX_CENTER_GAP2, t }) => {
  const [loading, setLoading] = useState(true);
  const [quotaData, setQuotaData] = useState(null);
  const [allocationData, setAllocationData] = useState(null);
  const [poolData, setPoolData] = useState(null);
  const [error, setError] = useState('');
  const refreshInFlightRef = useRef(false);
  const [refreshing, setRefreshing] = useState(false);

  const loadCodexQuotas = useCallback(async (forceRefresh = false) => {
    if (refreshInFlightRef.current) {
      return;
    }
    refreshInFlightRef.current = true;
    setRefreshing(true);
    setError('');
    try {
      const requests = [
        API.get('/api/external/codex-quotas', {
          params: forceRefresh ? { refresh: 1 } : undefined,
          disableDuplicate: true,
          skipErrorHandler: true,
        }),
        API.get('/api/external/codex-quota-allocation', {
          disableDuplicate: true,
          skipErrorHandler: true,
        }),
      ];
      if (isAdmin()) {
        requests.push(
          API.get('/api/external/codex-quota-pool', {
            disableDuplicate: true,
            skipErrorHandler: true,
          }),
        );
      }
      const [quotaResult, allocationResult, poolResult] =
        await Promise.allSettled(requests);

      if (
        quotaResult.status === 'fulfilled' &&
        quotaResult.value.data.success
      ) {
        setQuotaData(quotaResult.value.data.data);
      } else {
        const message =
          quotaResult.status === 'fulfilled'
            ? quotaResult.value.data.message
            : quotaResult.reason?.response?.data?.message ||
              quotaResult.reason?.message;
        setQuotaData(null);
        setError(message || t('Codex quota unavailable'));
      }
      if (
        allocationResult.status === 'fulfilled' &&
        allocationResult.value.data.success
      ) {
        setAllocationData(allocationResult.value.data.data);
      }
      setPoolData(
        poolResult?.status === 'fulfilled' && poolResult.value.data.success
          ? poolResult.value.data.data
          : null,
      );
    } finally {
      refreshInFlightRef.current = false;
      setLoading(false);
      setRefreshing(false);
    }
  }, [t]);

  useEffect(() => {
    loadCodexQuotas();
  }, [loadCodexQuotas]);

  useEffect(() => {
    if ((allocationData?.pending_weight ?? 0) <= 0) {
      return undefined;
    }
    const timer = window.setInterval(
      loadCodexQuotas,
      CODEX_PENDING_REFRESH_INTERVAL,
    );
    return () => window.clearInterval(timer);
  }, [allocationData?.pending_weight, loadCodexQuotas]);

  const items = quotaData?.items || [];

  return (
    <Card
      {...CARD_PROPS}
      className='shadow-sm !rounded-2xl'
      title={
        <div className='flex items-center justify-between w-full gap-2'>
          <div className={FLEX_CENTER_GAP2}>
            <Gauge size={16} />
            {t('Codex quota')}
          </div>
          <Button
            icon={<RefreshCw size={14} />}
            onClick={() => loadCodexQuotas(true)}
            loading={refreshing}
            size='small'
            theme='borderless'
            type='tertiary'
            aria-label={t('Refresh')}
            className='text-gray-500 hover:text-blue-500 hover:bg-blue-50 !rounded-full'
          />
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <div className='p-3'>
        {allocationData?.enabled && (
          <div className='mb-3 grid grid-cols-3 gap-2 rounded-xl border border-gray-100 p-2 text-center text-xs'>
            <div>
              <div className='text-gray-500'>{t('已分配')}</div>
              <div className='font-semibold tabular-nums'>
                {(allocationData.effective_bps / 100).toFixed(2)}%
              </div>
            </div>
            <div>
              <div className='text-gray-500'>{t('已确认使用')}</div>
              <div className='font-semibold tabular-nums'>
                {formatAllocatedUsagePercent(
                  allocationData.settled_used_units ??
                    allocationData.used_units,
                  allocationData.allocated_units,
                )}
              </div>
            </div>
            <div>
              <div className='text-gray-500'>{t('状态')}</div>
              <div className='font-semibold'>
                {allocationData.unattributed_weight > 0
                  ? t('需要核查')
                  : allocationData.stale
                    ? t('同步已过期')
                    : !allocationData.pool_available
                      ? t('Codex quota unavailable')
                      : allocationData.pending_weight > 0
                        ? t('等待中')
                        : t('正常')}
              </div>
            </div>
          </div>
        )}

        {allocationData?.unattributed_weight > 0 && (
          <div className='mb-3 text-center text-xs tabular-nums text-gray-500'>
            {t('待核查估算权重')}:{' '}
            {Number(allocationData.unattributed_weight).toLocaleString()}
          </div>
        )}

        {allocationData?.pending_weight > 0 && (
          <div className='mb-3 text-center text-xs tabular-nums text-gray-500'>
            {t('待结算估算权重')}:{' '}
            {Number(allocationData.pending_weight).toLocaleString()}
          </div>
        )}

        {isAdmin() && poolData && (
          <div className='mb-3 grid grid-cols-3 gap-2 rounded-xl border border-gray-100 p-2 text-center text-xs'>
            <div>
              <div className='text-gray-500'>{t('额度池剩余')}</div>
              <div className='font-semibold tabular-nums'>
                {poolData.pool_capacity_units > 0
                  ? `${(
                      (poolData.pool_remaining_units /
                        poolData.pool_capacity_units) *
                      100
                    ).toFixed(1)}%`
                  : '0%'}
              </div>
            </div>
            <div>
              <div className='text-gray-500'>{t('已分配份额')}</div>
              <div className='font-semibold tabular-nums'>
                {(poolData.allocated_bps / 100).toFixed(2)}%
              </div>
            </div>
            <div>
              <div className='text-gray-500'>{t('纳入凭证数')}</div>
              <div className='font-semibold tabular-nums'>
                {poolData.included_count}
              </div>
            </div>
          </div>
        )}

        {loading ? (
          <Skeleton
            loading
            active
            placeholder={
              <div className='space-y-3'>
                <Skeleton.Title active style={{ width: 120, height: 16 }} />
                <Skeleton.Paragraph active rows={3} />
              </div>
            }
          />
        ) : quotaData?.configured === false ? (
          <div className='rounded-xl bg-gray-50 p-3 text-sm text-gray-500'>
            {t('Codex quota not configured')}
          </div>
        ) : error ? (
          <div className='rounded-xl bg-red-50 p-3 text-sm text-red-500'>
            {t('Codex quota unavailable')}
          </div>
        ) : items.length > 0 ? (
          <ScrollableContainer maxHeight='20rem'>
            <div className='space-y-3'>
              {items.map((item, index) => (
                <CodexQuotaItem
                  key={index}
                  item={item}
                  itemIndex={index}
                  t={t}
                />
              ))}
            </div>
          </ScrollableContainer>
        ) : (
          <div className='rounded-xl bg-gray-50 p-3 text-sm text-gray-500'>
            {t('No Codex quota data')}
          </div>
        )}
      </div>
    </Card>
  );
};

export default CodexQuotaPanel;
