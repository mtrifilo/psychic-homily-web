'use client'

import { type MouseEvent, type ReactNode } from 'react'
import { ArrowDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { isPlainNavigationClick } from '@/components/shared/paginationChrome'
import {
  SETTINGS_SECTIONS,
  settingsSectionCount,
  type SettingsSection,
} from '../sections'

const COUNT_CLASS = 'font-mono text-[11px] text-muted-foreground'

function SectionCount({ section }: { section: SettingsSection }) {
  const count = settingsSectionCount(section)
  return (
    <>
      <span aria-hidden="true" className={COUNT_CLASS}>
        {count}
      </span>
      <span className="sr-only">
        ({count} {count === 1 ? 'setting' : 'settings'})
      </span>
    </>
  )
}

/**
 * A section link. The href keeps it a real link (open in a new tab, copy the
 * address); a plain click is handed to `onJump` instead of the browser's own
 * fragment navigation.
 */
function SectionLink({
  section,
  onJump,
  className,
  children,
  ...rest
}: {
  section: SettingsSection
  onJump: (anchor: string) => void
  className: string
  children: ReactNode
  'aria-current'?: 'true'
}) {
  const handleClick = (event: MouseEvent<HTMLAnchorElement>) => {
    if (!isPlainNavigationClick(event)) return
    event.preventDefault()
    onJump(section.anchor)
  }

  return (
    <a
      href={`#${section.anchor}`}
      onClick={handleClick}
      className={className}
      {...rest}
    >
      {children}
    </a>
  )
}

/** The desktop rail: every section in page order, marking `activeAnchor`. */
export function SettingsRail({
  activeAnchor,
  onJump,
  className,
}: {
  activeAnchor: string
  onJump: (anchor: string) => void
  className?: string
}) {
  return (
    <nav aria-label="Settings sections" className={className}>
      <ul className="flex flex-col gap-0.5">
        {SETTINGS_SECTIONS.map(section => {
          const isActive = section.anchor === activeAnchor
          return (
            <li key={section.anchor}>
              <SectionLink
                section={section}
                onJump={onJump}
                aria-current={isActive ? 'true' : undefined}
                className={cn(
                  'flex items-center justify-between gap-3 border-l-2 px-3 py-2 text-sm transition-colors',
                  isActive
                    ? 'border-primary bg-muted font-medium text-foreground'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                )}
              >
                <span>{section.title}</span>
                <SectionCount section={section} />
              </SectionLink>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}

/**
 * The mobile jump index: the same sections as the rail, as a list at the top
 * of the one long page.
 */
export function SettingsJumpIndex({
  onJump,
  className,
}: {
  onJump: (anchor: string) => void
  className?: string
}) {
  return (
    <nav
      aria-labelledby="settings-jump-index-label"
      className={cn('rounded-lg border border-border bg-card', className)}
    >
      <p
        id="settings-jump-index-label"
        className="px-4 pb-2 pt-3 font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground"
      >
        On this page
      </p>
      <ul className="divide-y divide-border border-t border-border">
        {SETTINGS_SECTIONS.map(section => (
          <li key={section.anchor}>
            <SectionLink
              section={section}
              onJump={onJump}
              className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm text-foreground hover:bg-muted/50"
            >
              <span>{section.title}</span>
              <span className="flex items-center gap-2">
                <SectionCount section={section} />
                <ArrowDown
                  aria-hidden="true"
                  className="h-3.5 w-3.5 text-muted-foreground"
                />
              </span>
            </SectionLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}
