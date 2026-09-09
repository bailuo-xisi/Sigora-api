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
import { Button } from '@douyinfe/semi-ui';
import BatchUserQuotaModal from './modals/BatchUserQuotaModal';

const UsersActions = ({
  setShowAddUser,
  selectedUsers,
  onBatchQuotaSuccess,
  t,
}) => {
  const [batchQuotaVisible, setBatchQuotaVisible] = useState(false);

  // Add new user
  const handleAddUser = () => {
    setShowAddUser(true);
  };

  return (
    <>
      <div className='flex gap-2 w-full md:w-auto order-2 md:order-1'>
        <Button
          className='flex-1 md:flex-initial'
          onClick={handleAddUser}
          size='small'
        >
          {t('添加用户')}
        </Button>
        <Button
          className='flex-1 md:flex-initial'
          type='primary'
          theme='outline'
          onClick={() => setBatchQuotaVisible(true)}
          disabled={selectedUsers.length === 0}
          size='small'
        >
          {t('批量调整额度')}
          {selectedUsers.length > 0 ? ` (${selectedUsers.length})` : ''}
        </Button>
      </div>

      <BatchUserQuotaModal
        visible={batchQuotaVisible}
        onCancel={() => setBatchQuotaVisible(false)}
        users={selectedUsers}
        onSuccess={onBatchQuotaSuccess}
      />
    </>
  );
};

export default UsersActions;
