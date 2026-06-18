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

import React, { useCallback, useEffect, useState } from 'react';
import { Button, Card, Skeleton, Tag } from '@douyinfe/semi-ui';
import { Gauge, RefreshCw } from 'lucide-react';
import { API } from '../../helpers';
import ScrollableContainer from '../common/ui/ScrollableContainer';

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

const formatPlanType = (planType) => {
  const trimmed = planType?.trim();
  if (!trimmed) return '';
  return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
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

const getQuotaBarClass = (remaining) => {
  if (remaining === null || remaining === undefined) return 'bg-gray-300';
  if (remaining <= 15) return 'bg-red-500';
  if (remaining <= 35) return 'bg-amber-500';
  return 'bg-emerald-500';
};

const CodexQuotaWindowRow = ({ quotaWindow, t }) => {
  const remaining = clampPercent(quotaWindow.remaining_percent);
  const resetTime = formatResetTime(quotaWindow.reset_at);

  return (
    <div className='space-y-1.5'>
      <div className='flex items-center justify-between gap-2 text-xs'>
        <span className='min-w-0 truncate font-medium text-gray-500'>
          {getWindowLabel(t, quotaWindow)}
        </span>
        <span
          className={`shrink-0 font-mono font-semibold tabular-nums ${getQuotaToneClass(
            remaining,
          )}`}
        >
          {formatPercent(remaining)}
        </span>
      </div>
      <div className='h-2 overflow-hidden rounded-full bg-gray-100'>
        <div
          className={`h-full rounded-full transition-all ${getQuotaBarClass(
            remaining,
          )}`}
          style={{ width: `${remaining ?? 0}%` }}
        />
      </div>
      <div className='truncate text-[11px] text-gray-400'>
        {resetTime
          ? t('Refreshes at {{time}}', { time: resetTime })
          : t('Reset time unavailable')}
      </div>
    </div>
  );
};

const CodexQuotaItem = ({ item, t }) => {
  const planType = formatPlanType(item.plan_type);
  return (
    <div className='rounded-xl border border-gray-100 bg-white p-3 shadow-sm'>
      <div className='mb-2 flex items-center justify-between gap-2'>
        <span className='min-w-0 truncate text-sm font-semibold text-gray-900'>
          {item.name || t('Codex auth file')}
        </span>
        {item.error ? (
          <Tag color='red' shape='circle' size='small'>
            {t('Error')}
          </Tag>
        ) : planType ? (
          <Tag color='blue' shape='circle' size='small'>
            {planType}
          </Tag>
        ) : null}
      </div>

      {item.error ? (
        <div className='truncate text-xs text-red-500' title={item.error}>
          {t('Quota unavailable')}
          {item.error_status ? ` (${item.error_status})` : ''}
        </div>
      ) : item.windows && item.windows.length > 0 ? (
        <div className='space-y-3'>
          {item.windows.map((quotaWindow) => (
            <CodexQuotaWindowRow
              key={`${item.name}-${quotaWindow.id}`}
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
  const [error, setError] = useState('');

  const loadCodexQuotas = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const res = await API.get('/api/external/codex-quotas', {
        disableDuplicate: true,
        skipErrorHandler: true,
      });
      const { success, message, data } = res.data;
      if (success) {
        setQuotaData(data);
      } else {
        setQuotaData(null);
        setError(message || t('Codex quota unavailable'));
      }
    } catch (err) {
      setQuotaData(null);
      setError(
        err?.response?.data?.message ||
          err?.message ||
          t('Codex quota unavailable'),
      );
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    loadCodexQuotas();
  }, [loadCodexQuotas]);

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
            onClick={loadCodexQuotas}
            loading={loading}
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
              {items.map((item) => (
                <CodexQuotaItem
                  key={`${item.name}-${item.auth_index}`}
                  item={item}
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
