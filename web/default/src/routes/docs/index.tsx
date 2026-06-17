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
import { createFileRoute } from '@tanstack/react-router'
import { BookOpen, ExternalLink, Wrench } from 'lucide-react'
import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

const resourceSections = [
  {
    title: '工具链接',
    description: '适合部署、同步、管理和日常维护的小工具。',
    icon: Wrench,
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
    icon: BookOpen,
    items: [],
  },
]

function DocsPage() {
  return (
    <PublicLayout>
      <div className='mx-auto flex max-w-5xl flex-col gap-8 pb-16 pt-6'>
        <section className='space-y-3'>
          <p className='text-muted-foreground text-sm font-medium'>Resources</p>
          <h1 className='text-3xl font-semibold tracking-normal md:text-4xl'>
            教程与工具
          </h1>
          <p className='text-muted-foreground max-w-2xl text-base leading-7'>
            这里会集中放一些实用教程和工具链接，方便需要的人快速找到可用资源。
          </p>
        </section>

        <div className='grid gap-8 md:grid-cols-2'>
          {resourceSections.map((section) => {
            const Icon = section.icon
            return (
              <section key={section.title} className='flex flex-col'>
                <div className='mb-4 flex items-start gap-3'>
                  <div className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-md'>
                    <Icon className='size-4' />
                  </div>
                  <div className='space-y-1'>
                    <h2 className='text-lg font-semibold'>{section.title}</h2>
                    <p className='text-muted-foreground text-sm leading-6'>
                      {section.description}
                    </p>
                  </div>
                </div>

                {section.items.length > 0 ? (
                  <div className='space-y-3'>
                    {section.items.map((item) => (
                      <article
                        key={item.href}
                        className='bg-muted/35 rounded-lg border p-4'
                      >
                        <div className='space-y-2'>
                          <h3 className='font-medium'>{item.title}</h3>
                          <p className='text-muted-foreground text-sm leading-6'>
                            {item.description}
                          </p>
                          <p className='text-muted-foreground break-all text-xs'>
                            {item.displayHref}
                          </p>
                        </div>
                        <Button
                          className='mt-4'
                          size='sm'
                          render={
                            <a
                              href={item.href}
                              target='_blank'
                              rel='noopener noreferrer'
                            />
                          }
                        >
                          打开工具
                          <ExternalLink className='size-3.5' />
                        </Button>
                      </article>
                    ))}
                  </div>
                ) : (
                  <div className='text-muted-foreground flex min-h-[132px] items-center rounded-lg border border-dashed p-4 text-sm'>
                    内容整理中。
                  </div>
                )}
              </section>
            )
          })}
        </div>
      </div>
    </PublicLayout>
  )
}

export const Route = createFileRoute('/docs/')({
  component: DocsPage,
})
