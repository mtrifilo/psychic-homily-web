'use client'

import { useCallback, useId, useRef, useState, type ReactNode } from 'react'
import { AlertCircle } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Input } from '@/components/ui/input'
import {
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from '@/features/auth/hooks/useCalendarFeed'
import { queryKeys } from '@/lib/queryClient'
import { cn } from '@/lib/utils'
import type { CalendarTokenStatusResponse } from '@/features/shows/types'
import {
  PRIVATE_LINK_CLASS,
  PanelError,
  PanelErrorWithRetry,
  PanelNote,
  holdFocusInPanel,
} from './CalendarPanelParts'
import { copyText } from './calendarPanelBrowser'
import { PANEL_COPY } from './savedShowsCalendarCopy'
import { SETTINGS_CALENDAR_COPY as COPY } from './settingsCalendarCopy'
import { CalendarCardButton } from './SettingsCalendarParts'
import { statusOfNewLink, type CalendarLinks } from './useSettingsCalendar'

type Confirming = 'new_link' | 'turn_off'

interface SettingsCalendarLinksProps {
  /** The two addresses, or null when the link's address cannot be shown. */
  links: CalendarLinks | null
  /** The link was replaced or turned off. */
  onLinkChanged: () => void
}

/**
 * The links block: the only place on the site an address is rendered as
 * text. Each field and Copy carries PRIVATE_LINK_CLASS. New link and Turn off
 * each ask first, inline, and act on both addresses, which one key makes.
 *
 * Every write puts the status it answered with in the cache, so the card
 * changes at once; the mutation hooks then refetch every calendar read.
 */
export function SettingsCalendarLinks({
  links,
  onLinkChanged,
}: SettingsCalendarLinksProps) {
  const queryClient = useQueryClient()
  const createLink = useCreateCalendarToken()
  const turnOffLink = useDeleteCalendarToken()
  const [confirming, setConfirming] = useState<Confirming | null>(null)
  // The control that opened a confirmation, focused again when the controls
  // return (Cancel, or a new link made).
  const returnFocusTo = useRef<Confirming | null>(null)
  const calendarFieldId = useId()
  const followsFieldId = useId()
  const blockRef = useRef<HTMLDivElement>(null)

  const open = (which: Confirming) => {
    createLink.reset()
    turnOffLink.reset()
    setConfirming(which)
  }
  const cancel = () => {
    returnFocusTo.current = confirming
    setConfirming(null)
  }

  const makeNewLink = () => {
    createLink.mutate(undefined, {
      onSuccess: created => {
        queryClient.setQueryData(
          queryKeys.calendar.tokenStatus,
          statusOfNewLink(created)
        )
        returnFocusTo.current = 'new_link'
        setConfirming(null)
        onLinkChanged()
      },
    })
  }

  const turnOff = () => {
    turnOffLink.mutate(undefined, {
      onSuccess: () => {
        // This block disappears with the link.
        if (blockRef.current) holdFocusInPanel(blockRef.current)
        const off: CalendarTokenStatusResponse = { has_token: false }
        queryClient.setQueryData(queryKeys.calendar.tokenStatus, off)
        onLinkChanged()
      },
    })
  }

  const focusWhenReturning = (which: Confirming) => (el: HTMLElement | null) => {
    if (el && returnFocusTo.current === which) {
      returnFocusTo.current = null
      el.focus()
    }
  }

  return (
    <div ref={blockRef} className="flex flex-col gap-3">
      {links ? (
        <>
          <LinkField
            key={links.feedUrl}
            id={calendarFieldId}
            label={COPY.calendarLinkLabel}
            address={links.feedUrl}
            copiedLine={PANEL_COPY.copied}
          />
          <LinkField
            key={links.followsFeedUrl}
            id={followsFieldId}
            label={
              <>
                {COPY.followsLinkLabel}{' '}
                <span className="font-normal text-muted-foreground">
                  {COPY.followsLinkDetail}
                </span>
              </>
            }
            address={links.followsFeedUrl}
            copiedLine={COPY.followsCopied}
          />
        </>
      ) : null}

      {confirming === 'new_link' ? (
        <InlineConfirmation
          sentence={COPY.confirmNewLink}
          action={COPY.confirmNewLinkAction}
          working={createLink.isPending}
          onConfirm={makeNewLink}
          onCancel={cancel}
          failure={
            createLink.isError ? (
              <PanelErrorWithRetry onRetry={makeNewLink}>
                {PANEL_COPY.createFailed}
              </PanelErrorWithRetry>
            ) : null
          }
        />
      ) : confirming === 'turn_off' ? (
        <InlineConfirmation
          sentence={COPY.confirmTurnOff}
          action={COPY.confirmTurnOffAction}
          working={turnOffLink.isPending}
          onConfirm={turnOff}
          onCancel={cancel}
          failure={
            turnOffLink.isError ? (
              <PanelErrorWithRetry onRetry={turnOff}>
                {COPY.turnOffFailed}
              </PanelErrorWithRetry>
            ) : null
          }
        />
      ) : (
        <div className="flex flex-col gap-2 sm:flex-row">
          <CalendarCardButton
            ref={focusWhenReturning('new_link')}
            onClick={() => open('new_link')}
          >
            {COPY.newLink}
          </CalendarCardButton>
          <CalendarCardButton
            ref={focusWhenReturning('turn_off')}
            onClick={() => open('turn_off')}
            className="text-destructive hover:text-destructive dark:hover:text-destructive"
          >
            {COPY.turnOff}
          </CalendarCardButton>
        </div>
      )}

      <p className="flex items-start gap-2 text-xs leading-4 text-muted-foreground">
        <AlertCircle className="size-3.5 shrink-0" aria-hidden="true" />
        {COPY.note}
      </p>
    </div>
  )
}

/**
 * One address: its label, the read-only field and Copy. The field is set
 * right to left with left-aligned text, so a field too narrow for the whole
 * address shows its END, the part that tells the two apart.
 */
function LinkField({
  id,
  label,
  address,
  copiedLine,
}: {
  id: string
  label: ReactNode
  address: string
  copiedLine: string
}) {
  const [copied, setCopied] = useState<boolean | null>(null)
  const copy = () => {
    setCopied(null)
    copyText(address).then(
      () => setCopied(true),
      () => setCopied(false)
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium leading-5 text-foreground">
        {label}
      </label>
      <div className="flex gap-2">
        <Input
          id={id}
          readOnly
          value={address}
          onFocus={event => event.currentTarget.select()}
          className={cn(
            'h-11 text-left font-code text-xs [direction:rtl] sm:h-8 md:text-xs',
            PRIVATE_LINK_CLASS
          )}
          data-testid="calendar-link-field"
        />
        <CalendarCardButton
          onClick={copy}
          className={cn('w-auto shrink-0 px-4 sm:w-auto', PRIVATE_LINK_CLASS)}
        >
          {COPY.copy}
        </CalendarCardButton>
      </div>
      <div aria-live="polite" className="flex flex-col not-empty:mt-0.5">
        {copied === true ? <PanelNote>{copiedLine}</PanelNote> : null}
        {copied === false ? <PanelError>{COPY.copyFailed}</PanelError> : null}
      </div>
    </div>
  )
}

/**
 * Asks before a change that disconnects every calendar and feed reader, in
 * the card itself rather than a dialog. Focus moves to the question when it
 * opens, so a screen reader reads it before the buttons.
 */
function InlineConfirmation({
  sentence,
  action,
  working,
  onConfirm,
  onCancel,
  failure,
}: {
  sentence: string
  action: string
  working: boolean
  onConfirm: () => void
  onCancel: () => void
  failure: ReactNode
}) {
  const sentenceId = useId()
  // Stable, so it focuses once, as the confirmation opens.
  const focusOnOpen = useCallback((el: HTMLDivElement | null) => el?.focus(), [])
  return (
    <div
      role="group"
      aria-labelledby={sentenceId}
      tabIndex={-1}
      ref={focusOnOpen}
      className="flex flex-col gap-2 rounded-sm focus:outline-none"
    >
      <p id={sentenceId} className="text-sm leading-5 text-foreground">
        {sentence}
      </p>
      <div className="flex flex-col gap-2 sm:flex-row">
        <CalendarCardButton
          variant="destructive"
          working={working}
          onClick={onConfirm}
        >
          {action}
        </CalendarCardButton>
        <CalendarCardButton onClick={onCancel} disabled={working}>
          {COPY.cancel}
        </CalendarCardButton>
      </div>
      {failure}
    </div>
  )
}
