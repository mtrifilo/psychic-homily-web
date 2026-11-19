'use client'

import { createContext, useContext, type ReactNode } from 'react'
import Link from 'next/link'
import type { LucideIcon } from 'lucide-react'
import { CALENDAR_FEED_SETTINGS_HREF } from '@/features/auth/components/settings/calendar-feed-anchor'
import { cn } from '@/lib/utils'
import { PANEL_COPY } from './savedShowsCalendarCopy'

/**
 * Where the panel is drawn. The sheet gives every row a 44px tap target and
 * stacks rows edge to edge; the popover spaces 20px rows 10px apart.
 */
export type CalendarPanelLayout = 'popover' | 'sheet'

export const CalendarPanelLayoutContext =
  createContext<CalendarPanelLayout>('popover')

export function useCalendarPanelLayout(): CalendarPanelLayout {
  return useContext(CalendarPanelLayoutContext)
}

/** The rows' stack: spaced in the popover, edge to edge in the sheet. */
export function rowStackClass(layout: CalendarPanelLayout): string {
  return layout === 'sheet' ? 'flex flex-col' : 'flex flex-col gap-2.5'
}

const ROW_CLASS =
  'inline-flex items-center gap-1.5 self-start rounded-sm text-left text-sm font-medium text-link hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50'

function rowClass(layout: CalendarPanelLayout, className?: string) {
  return cn(ROW_CLASS, layout === 'sheet' && 'min-h-11', className)
}

interface RowButtonProps {
  children: ReactNode
  onClick: () => void
  icon?: LucideIcon
  disabled?: boolean
  /**
   * The row acts on the private calendar link. Kept out of analytics
   * autocapture, which would otherwise record the click with its context.
   */
  handlesLink?: boolean
  testId?: string
}

/** A row that does something in the browser: a text link in looks, a button in behaviour. */
export function PanelRowButton({
  children,
  onClick,
  icon: Icon,
  disabled,
  handlesLink,
  testId,
}: RowButtonProps) {
  const layout = useCalendarPanelLayout()
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      data-testid={testId}
      className={rowClass(layout, handlesLink ? 'ph-no-capture' : undefined)}
    >
      {children}
      {Icon ? <Icon className="size-3.5 shrink-0" aria-hidden="true" /> : null}
    </button>
  )
}

/** A row that is a plain link to a page or file. */
export function PanelRowLink({
  href,
  children,
  icon: Icon,
  onClick,
}: {
  href: string
  children: ReactNode
  icon?: LucideIcon
  onClick?: () => void
}) {
  const layout = useCalendarPanelLayout()
  return (
    <a href={href} onClick={onClick} className={rowClass(layout)}>
      {children}
      {Icon ? <Icon className="size-3.5 shrink-0" aria-hidden="true" /> : null}
    </a>
  )
}

/** "Manage in Settings": the one way into the Settings calendar card. */
export function ManageInSettingsLink({ onNavigate }: { onNavigate: () => void }) {
  const layout = useCalendarPanelLayout()
  return (
    <Link
      href={CALENDAR_FEED_SETTINGS_HREF}
      onClick={onNavigate}
      className={rowClass(layout)}
    >
      {PANEL_COPY.manageInSettings}
    </Link>
  )
}

/** A muted 12px line: a caption, a status line, a confirmation. */
export function PanelNote({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <p className={cn('text-xs leading-4 text-muted-foreground', className)}>
      {children}
    </p>
  )
}

/** An inline failure line under the row that failed. */
export function PanelError({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="text-xs leading-4 text-destructive">
      {children}
    </p>
  )
}

/** A block's plain title, for a phone block that is not itself a row. */
export function PanelBlockTitle({ children }: { children: ReactNode }) {
  return (
    <p className="pt-3 text-sm font-medium leading-5 text-foreground">
      {children}
    </p>
  )
}
