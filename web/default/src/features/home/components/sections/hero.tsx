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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const endpoint = 'https://sigora.top/v1/responses'

  const copyEndpoint = () => {
    void navigator.clipboard?.writeText(endpoint)
  }

  const renderDocsButton = () => {
    return (
      <Button
        variant='outline'
        size='lg'
        className='border-white/55 bg-white/45 text-slate-900 shadow-sm backdrop-blur-md hover:bg-white/70'
        render={<Link to='/docs' />}
      >
        <BookOpen data-icon='inline-start' />
        <span>{t('Docs')}</span>
      </Button>
    )
  }

  return (
    <section className='sigora-hero relative flex min-h-svh overflow-hidden px-6 pt-24 pb-8 md:pt-28 md:pb-10'>
      <img
        aria-hidden
        src='/sigora-hero-anime.jpg'
        alt=''
        className='absolute inset-0 size-full object-cover object-[58%_center]'
      />
      <div aria-hidden className='sigora-hero-overlay absolute inset-0' />
      <div aria-hidden className='sigora-hero-lines absolute inset-0' />

      <div className='relative mx-auto flex w-full max-w-7xl flex-1 items-end'>
        <div className='mb-8 flex max-w-2xl flex-col items-start text-left md:mb-12'>
          <div
            className='landing-animate-fade-up mb-4 inline-flex items-center gap-2 rounded-full border border-white/55 bg-white/45 px-3 py-1.5 text-xs font-medium text-slate-900 opacity-0 shadow-sm backdrop-blur-md'
            style={{ animationDelay: '0ms' }}
          >
            <span className='relative flex size-1.5'>
              <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-sky-400 opacity-75' />
              <span className='relative inline-flex size-1.5 rounded-full bg-sky-500' />
            </span>
            <span>{t('AI Application Infrastructure Foundation')}</span>
          </div>

          <h1
            className='landing-animate-fade-up text-6xl leading-none font-black tracking-normal opacity-0 sm:text-7xl md:text-8xl'
            style={{ animationDelay: '60ms' }}
          >
            <span className='sigora-blue-art-text'>Sigora</span>
          </h1>
          <p
            className='landing-animate-fade-up mt-5 max-w-xl text-base leading-relaxed text-slate-900/78 opacity-0 md:text-lg'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'Access a vast selection of models via a standard, unified API protocol. Power AI applications, manage digital assets, and connect the Future.'
            )}
          </p>

          <div
            className='landing-animate-fade-up mt-6 flex max-w-full flex-wrap items-center overflow-hidden rounded-lg border border-white/55 bg-white/50 text-sm text-slate-900 opacity-0 shadow-sm backdrop-blur-md sm:flex-nowrap sm:rounded-full sm:text-base'
            style={{ animationDelay: '160ms' }}
          >
            <span className='px-4 py-2.5 font-medium whitespace-nowrap'>
              https://sigora.top
            </span>
            <span className='border-l border-white/60 px-4 py-2.5 font-semibold text-blue-700'>
              /v1/responses
            </span>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='mr-1 rounded-full text-slate-700 hover:bg-white/60'
              onClick={copyEndpoint}
              aria-label='Copy endpoint'
            >
              <Copy />
            </Button>
          </div>

          <div
            className='landing-animate-fade-up mt-8 flex flex-wrap items-center gap-3 opacity-0'
            style={{ animationDelay: '220ms' }}
          >
            {props.isAuthenticated ? (
              <>
                <Button
                  size='lg'
                  className='bg-blue-600 text-white shadow-[0_10px_28px_rgba(37,99,235,0.28)] hover:bg-blue-500'
                  render={<Link to='/dashboard' />}
                >
                  {t('Go to Dashboard')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                {renderDocsButton()}
              </>
            ) : (
              <>
                <Button
                  size='lg'
                  className='bg-blue-600 text-white shadow-[0_10px_28px_rgba(37,99,235,0.28)] hover:bg-blue-500'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                <Button
                  variant='outline'
                  size='lg'
                  className='border-white/55 bg-white/45 text-slate-900 shadow-sm backdrop-blur-md hover:bg-white/70'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
                {renderDocsButton()}
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
