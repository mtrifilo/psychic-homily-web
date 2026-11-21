'use client'

import { useCallback, useId, useRef, useState, type ReactNode } from 'react'
import { AlertCircle } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Input } from '@/components/ui/input'
import {
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from '@/features/auth/hooks/useCalendarFeed'
import { cn } from '@/lib/utils'
import type { CalendarTokenStatusResponse } from '@/features/shows/types'
import {
  PRIVATE_LINK_CLASS,
  PanelError,
  PanelErrorWithRetry,
  PanelNote,
} from './CalendarPanelParts'
import { copyText } from './calendarPanelBrowser'
import { PANEL_COPY } from './savedShowsCalendarCopy'
import { SETTINGS_CALENDAR_COPY as COPY } from './settingsCalendarCopy'
import {
  CalendarCardButton,
  useArmedAfterMount,
  useHoldFocusOnLeave,
} from './SettingsCalendarParts'
import {
  showCalendarStatus,
  statusOfNewLink,
  type CalendarLinks,
} from './useSettingsCalendar'

type Confirming = 'new_link' | 'turn_off'

interface SettingsCalendarLinksProps {
  /** The two addresses, or null when the link's address cannot be shown. */
  links: CalendarLinks | null
  /** A New link or Turn off request settled, whatever its outcome. */
  onLinkWritten: () => void
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
  onLinkWritten,
}: SettingsCalendarLinksProps) {
  const queryClient = useQueryClient()
  const createLink = useCreateCalendarToken()
  const turnOffLink = useDeleteCalendarToken()
  // Which confirmation is open, and for which address. A confirmation asked
  // about one link closes when the address changes, including by a refetch
  // after a write whose response was lost.
  const [asked, setAsked] = useState<{
    which: Confirming
    address: string | null
  } | null>(null)
  const currentAddress = links?.feedUrl ?? null
  const confirming =
    asked && asked.address === currentAddress ? asked.which : null
  // The control that opened a confirmation, focused again when the controls
  // return (Cancel, or a new link made).
  const returnFocusTo = useRef<Confirming | null>(null)
  const calendarFieldId = useId()
  const followsFieldId = useId()
  // The block leaves whenever the link does, also when a refetch finds it
  // gone (turned off elsewhere, or a turn-off whose answer was lost).
  const blockRef = useHoldFocusOnLeave<HTMLDivElement>()

  const open = (which: Confirming) => {
    createLink.reset()
    turnOffLink.reset()
    setAsked({ which, address: currentAddress })
  }
  const cancel = () => {
    returnFocusTo.current = confirming
    setAsked(null)
  }

  const makeNewLink = () => {
    createLink.mutate(undefined, {
      onSuccess: created => {
        showCalendarStatus(queryClient, statusOfNewLink(created))
        setAsked(null)
      },
      onSettled: () => {
        // Also after a failure: a create whose answer was lost may have
        // replaced the link, which closes the question.
        returnFocusTo.current = 'new_link'
        onLinkWritten()
      },
    })
  }

  const turnOff = () => {
    turnOffLink.mutate(undefined, {
      onSuccess: () => {
        const off: CalendarTokenStatusResponse = { has_token: false }
        showCalendarStatus(queryClient, off)
      },
      onSettled: onLinkWritten,
    })
  }

  // Returns focus to the control that opened a question, but only from
  // inside the card (or from nowhere): a question that closes later, after
  // the person moved on to another card, must not pull focus back.
  const focusWhenReturning = (which: Confirming) => (el: HTMLElement | null) => {
    if (!el || returnFocusTo.current !== which) return
    returnFocusTo.current = null
    const active = document.activeElement
    const card = el.closest('[data-calendar-surface]')
    if (!active || active === document.body || card?.contains(active)) {
      el.focus()
    }
  }

  return (
    <div ref={blockRef} className="flex flex-col gap-3">
      {links ? (
        <>
          <LinkField
            id={calendarFieldId}
            label={COPY.calendarLinkLabel}
            address={links.feedUrl}
            copiedLine={PANEL_COPY.copied}
          />
          <LinkField
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
 * address shows its END, the part that tells the two apart, with an ellipsis
 * where the beginning is cut (a browser check covers what it draws).
 *
 * The field stays mounted when its address is replaced, so focus on it or on
 * Copy stays put; a copy's note belongs to the address it was made for and is
 * not shown for a new one.
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
  const [copyResult, setCopyResult] = useState<{
    address: string
    copied: boolean
  } | null>(null)
  const copied = copyResult?.address === address ? copyResult.copied : null
  const copy = () => {
    setCopyResult(null)
    copyText(address).then(
      () => setCopyResult({ address, copied: true }),
      () => setCopyResult({ address, copied: false })
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
            'h-11 text-left text-ellipsis font-code text-xs [direction:rtl] sm:h-8 md:text-xs',
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
 * How long a confirmation's destructive button ignores activation, by pointer
 * or key, after the confirmation appears. Below `sm` the buttons stack full
 * width, so the confirming button can sit where the control that opened the
 * question was, and without the wait the second tap of a double tap would
 * confirm.
 */
export const CONFIRM_ARMING_DELAY_MS = 400

/**
 * Asks before a change that disconnects every calendar and feed reader, in
 * the card itself rather than a dialog. Focus moves to the question when it
 * opens, so a screen reader reads it before the buttons. The destructive
 * button acts only once CONFIRM_ARMING_DELAY_MS has passed; Cancel always acts.
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
  const isArmed = useArmedAfterMount(CONFIRM_ARMING_DELAY_MS)
  // The question can close under focus when a refetch finds the address
  // changed (another window, or a write whose answer was lost).
  const leaveRef = useHoldFocusOnLeave<HTMLDivElement>()
  // Stable, so it focuses once, as the confirmation opens.
  const focusOnOpen = useCallback(
    (el: HTMLDivElement | null) => {
      leaveRef.current = el
      el?.focus()
    },
    [leaveRef]
  )
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
          onClick={() => {
            if (isArmed()) onConfirm()
          }}
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
