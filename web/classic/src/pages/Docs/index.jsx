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

import React from 'react';
import { Button } from '@douyinfe/semi-ui';
import { IconExternalOpen } from '@douyinfe/semi-icons';

const resourceSections = [
  {
    title: '工具链接',
    description: '适合部署、同步、管理和日常维护的小工具。',
    items: [
      {
        title: 'Codex 对话同步项目工具',
        description:
          '用于同步和整理 Codex 对话记录，给有需要的人快速接入自己的工作流。',
        href: 'https://github.com/bailuo-xisi/codex-history-sync-tool.git',
        displayHref: 'bailuo-xisi/codex-history-sync-tool.git',
      },
    ],
  },
  {
    title: '教程',
    description: '后续可以放部署教程、使用说明和常见问题。',
    items: [],
  },
];

const Docs = () => {
  return (
    <div className='classic-page-fill pt-[88px] px-4 pb-12'>
      <div className='mx-auto flex max-w-5xl flex-col gap-8'>
        <section className='space-y-3'>
          <p className='text-sm font-semibold text-semi-color-tertiary'>
            Resources
          </p>
          <h1 className='text-3xl md:text-4xl font-semibold'>教程与工具</h1>
          <p className='max-w-2xl text-base leading-7 text-semi-color-secondary'>
            这里会集中放一些实用教程和工具链接，方便需要的人快速找到可用资源。
          </p>
        </section>

        <div className='grid gap-8 md:grid-cols-2'>
          {resourceSections.map((section) => (
            <section key={section.title} className='flex flex-col'>
              <div className='mb-4 space-y-1'>
                <h2 className='text-lg font-semibold'>{section.title}</h2>
                <p className='text-sm leading-6 text-semi-color-secondary'>
                  {section.description}
                </p>
              </div>

              {section.items.length > 0 ? (
                <div className='space-y-3'>
                  {section.items.map((item) => (
                    <article
                      key={item.href}
                      className='rounded-lg border border-semi-color-border bg-semi-color-fill-0 p-4'
                    >
                      <div className='space-y-2'>
                        <h3 className='font-medium'>{item.title}</h3>
                        <p className='text-sm leading-6 text-semi-color-secondary'>
                          {item.description}
                        </p>
                        <p className='break-all text-xs text-semi-color-tertiary'>
                          {item.displayHref}
                        </p>
                      </div>
                      <Button
                        className='mt-4'
                        size='small'
                        type='primary'
                        icon={<IconExternalOpen />}
                        onClick={() =>
                          window.open(item.href, '_blank', 'noopener,noreferrer')
                        }
                      >
                        打开工具
                      </Button>
                    </article>
                  ))}
                </div>
              ) : (
                <div className='flex min-h-[132px] items-center rounded-lg border border-dashed border-semi-color-border p-4 text-sm text-semi-color-tertiary'>
                  内容整理中。
                </div>
              )}
            </section>
          ))}
        </div>
      </div>
    </div>
  );
};

export default Docs;
