'use client'

import { createContext, useContext, type ReactNode } from 'react'
import type React from 'react'
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

/**
 * On every element that carries or acts on the private calendar link. The
 * address must never reach a recorder: PostHog skips `ph-no-capture` in
 * autocapture and its replay, and Sentry Session Replay blanks `sentry-block`
 * (it ignores PostHog's class), so an href holding the token is never
 * uploaded with a replay.
 */
export const PRIVATE_LINK_CLASS = 'ph-no-capture sentry-block'

const CALENDAR_SURFACE_ATTRIBUTE = 'data-calendar-surface'

/**
 * Spread on the element that holds a calendar surface (the panel's popover or
 * sheet, the steps page): the element holdFocusInSurface returns focus to.
 */
export const CALENDAR_SURFACE_PROPS = { [CALENDAR_SURFACE_ATTRIBUTE]: '' }

/**
 * Moves focus to the calendar surface itself before a control that is about
 * to unmount (a retry line, a stopped app's block) acts. The popover does not
 * trap focus, so focus on a removed control would drop to the page's body and
 * leave the person outside the surface they are still using.
 */
export function holdFocusInSurface(control: HTMLElement) {
  control.closest<HTMLElement>(`[${CALENDAR_SURFACE_ATTRIBUTE}]`)?.focus()
}

/** The rows' stack: spaced in the popover, edge to edge in the sheet. */
export function rowStackClass(layout: CalendarPanelLayout): string {
  return layout === 'sheet' ? 'flex flex-col' : 'flex flex-col gap-2.5'
}

/** How a calendar link reads, in a row or inside a sentence. */
export const CALENDAR_LINK_TEXT_CLASS =
  'rounded-sm font-medium text-link hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

const ROW_CLASS = cn(
  'inline-flex items-center gap-1.5 self-start text-left text-sm disabled:opacity-50',
  CALENDAR_LINK_TEXT_CLASS
)

function rowClass(layout: CalendarPanelLayout, className?: string) {
  return cn(ROW_CLASS, layout === 'sheet' && 'min-h-11', className)
}

interface RowButtonProps {
  children: ReactNode
  onClick: (event: React.MouseEvent<HTMLButtonElement>) => void
  icon?: LucideIcon
  disabled?: boolean
  /** The row acts on the private calendar link; see PRIVATE_LINK_CLASS. */
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
      className={rowClass(layout, handlesLink ? PRIVATE_LINK_CLASS : undefined)}
    >
      {children}
      {Icon ? <Icon className="size-3.5 shrink-0" aria-hidden="true" /> : null}
    </button>
  )
}

/**
 * A row that is a link: a file, or an address a calendar app or another site
 * takes over. `newTab` opens it in a new tab with no opener and no referrer.
 */
export function PanelRowLink({
  href,
  children,
  icon: Icon,
  onClick,
  newTab,
  handlesLink,
}: {
  href: string
  children: ReactNode
  icon?: LucideIcon
  onClick?: () => void
  newTab?: boolean
  /** The href carries the private calendar link; see PRIVATE_LINK_CLASS. */
  handlesLink?: boolean
}) {
  const layout = useCalendarPanelLayout()
  return (
    <a
      href={href}
      onClick={onClick}
      {...(newTab ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
      className={rowClass(layout, handlesLink ? PRIVATE_LINK_CLASS : undefined)}
    >
      {children}
      {Icon ? <Icon className="size-3.5 shrink-0" aria-hidden="true" /> : null}
    </a>
  )
}

/**
 * "Manage in Settings": the one way into the Settings calendar card.
 * `onNavigate` closes a panel the link leaves; a page has nothing to close.
 */
export function ManageInSettingsLink({ onNavigate }: { onNavigate?: () => void }) {
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

const TRY_AGAIN = 'Try again.'

/** Whether a failure line ends with the words that are its own retry. */
export function endsWithRetry(line: string): boolean {
  return line.endsWith(TRY_AGAIN)
}

/**
 * A failure line that ends "Try again.", where those two words are the retry
 * control, a link inside the sentence, so the line reads exactly as written.
 * A line without those words renders whole, with no retry; the copy test
 * keeps every retry line ending with them.
 */
export function PanelErrorWithRetry({
  children,
  onRetry,
  retrying = false,
}: {
  children: string
  onRetry: () => void
  /** A retry is in flight; the control waits rather than sending another. */
  retrying?: boolean
}) {
  const layout = useCalendarPanelLayout()
  if (!endsWithRetry(children)) return <PanelError>{children}</PanelError>
  const lead = children.slice(0, -TRY_AGAIN.length)
  return (
    <p role="alert" className="text-xs leading-4 text-destructive">
      {lead}
      <button
        type="button"
        disabled={retrying}
        onClick={event => {
          holdFocusInSurface(event.currentTarget)
          onRetry()
        }}
        className={cn(
          'rounded-sm underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50',
          // The sheet's 44px tap target, without changing how the line reads.
          layout === 'sheet' &&
            "relative after:absolute after:-inset-x-1 after:-inset-y-4 after:content-['']"
        )}
      >
        {TRY_AGAIN}
      </button>
    </p>
  )
}

/**
 * The link could not be loaded, or made, so nothing that needs it can act:
 * the line for the step that failed, ending in its retry.
 */
export function LinkFailedLine({
  failure,
  onRetry,
}: {
  failure: 'load' | 'create'
  onRetry: () => void
}) {
  return (
    <PanelErrorWithRetry onRetry={onRetry}>
      {failure === 'create' ? PANEL_COPY.createFailed : PANEL_COPY.loadFailed}
    </PanelErrorWithRetry>
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
