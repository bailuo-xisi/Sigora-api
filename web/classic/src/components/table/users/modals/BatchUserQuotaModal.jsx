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

import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Typography,
} from '@douyinfe/semi-ui';
import {
  API,
  getCurrencyConfig,
  showError,
  showSuccess,
} from '../../../../helpers';
import { displayAmountToQuota } from '../../../../helpers/quota';

const BatchUserQuotaModal = ({ visible, onCancel, users, onSuccess }) => {
  const { t } = useTranslation();
  const [mode, setMode] = useState('add');
  const [amount, setAmount] = useState('');
  const [loading, setLoading] = useState(false);

  const currencyConfig = getCurrencyConfig();
  const numericAmount = Number(amount);
  const quotaValue = amount === '' ? 0 : displayAmountToQuota(amount);
  const validAmount =
    amount !== '' &&
    Number.isFinite(numericAmount) &&
    (mode === 'override' || quotaValue > 0);

  const reset = () => {
    setMode('add');
    setAmount('');
  };

  const handleCancel = () => {
    if (loading) return;
    reset();
    onCancel();
  };

  const handleConfirm = async () => {
    if (!validAmount || users.length === 0) return;

    setLoading(true);
    try {
      const res = await API.post('/api/user/batch/quota', {
        ids: users.map((user) => user.id),
        mode,
        value: mode === 'override' ? quotaValue : Math.abs(quotaValue),
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message || t('操作失败，请重试'));
        return;
      }

      showSuccess(t('调整额度成功'));
      reset();
      onCancel();
      await onSuccess();
    } catch (error) {
      showError(error?.message || t('操作失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      centered
      visible={visible}
      title={`${t('批量调整额度')} (${users.length})`}
      onOk={handleConfirm}
      onCancel={handleCancel}
      confirmLoading={loading}
      okButtonProps={{ disabled: !validAmount || users.length === 0 }}
      cancelButtonProps={{ disabled: loading }}
    >
      <div className='space-y-4'>
        <Typography.Text type='secondary'>
          {t('对选中的 {{count}} 个用户应用相同的额度调整。', {
            count: users.length,
          })}
        </Typography.Text>

        <div>
          <div className='mb-1'>
            <Typography.Text size='small'>{t('操作')}</Typography.Text>
          </div>
          <RadioGroup
            type='button'
            value={mode}
            onChange={(event) => {
              setMode(event.target.value);
              setAmount('');
            }}
          >
            <Radio value='add'>{t('添加')}</Radio>
            <Radio value='subtract'>{t('减少')}</Radio>
            <Radio value='override'>{t('覆盖')}</Radio>
          </RadioGroup>
        </div>

        <div>
          <div className='mb-1'>
            <Typography.Text size='small'>
              {currencyConfig.type === 'TOKENS' ? t('额度') : t('金额')}
            </Typography.Text>
          </div>
          <InputNumber
            prefix={
              currencyConfig.type === 'TOKENS'
                ? undefined
                : currencyConfig.symbol
            }
            placeholder={
              currencyConfig.type === 'TOKENS' ? t('输入额度') : t('输入金额')
            }
            value={amount}
            precision={currencyConfig.type === 'TOKENS' ? 0 : 6}
            min={mode === 'override' ? undefined : 0}
            step={currencyConfig.type === 'TOKENS' ? 1 : 0.000001}
            onChange={(value) => {
              setAmount(value === '' || value == null ? '' : value);
            }}
            style={{ width: '100%' }}
            showClear
          />
        </div>
      </div>
    </Modal>
  );
};

export default BatchUserQuotaModal;
