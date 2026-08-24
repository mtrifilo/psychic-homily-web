'use client'

import { useState } from 'react'
import { BracketLink } from './BracketLink'
import { HomeMetroSelect, useHomeMetroLabel } from './HomeMetroField'
import { useAlertPreferences } from '@/features/auth/hooks/useAlertPreferences'
import {
  CUSTOM_ALERTS_HREF,
  followAlertHasScopeAxis,
  followAlertPendingNote,
} from './followAlertChoices'

interface LibraryAlertsBarProps {
  /** PLURAL follow path segment for the tab this bar sits above. */
  entityType: string
}

/**
 * The Library's alerts context bar (PSY-1905).
 *
 * Sits above the follow rows on the tabs whose follows carry an alert
 * subscription. It answers the two questions the per-row brackets raise and
 * cannot answer themselves: what a new follow starts at, and what "near me"
 * currently means.
 *
 * The starting scope is stated rather than offered as a control. Near-me is
 * the SHIPPED default for a new artist follow (PSY-1893), resolved server-side
 * rather than stored, and there is no account-level scope preference to write:
 * the shipped alert matrix pins channels only. Drawing pickable chips here
 * would be a control with nowhere to save to.
 */
export function LibraryAlertsBar({ entityType }: LibraryAlertsBarProps) {
  // A venue sits in one place. Its follows have no scope axis at all, so the
  // starting-scope sentence and the home area would be explaining a
  // restriction this tab's follows do not have, and contradicting the venue
  // control one page over that says exactly that.
  const hasScopeAxis = followAlertHasScopeAxis(entityType)
  const pendingNote = followAlertPendingNote(entityType)

  const {
    data: preferences,
    isError,
    refetch,
  } = useAlertPreferences(hasScopeAxis)
  const [isEditingArea, setIsEditingArea] = useState(false)
  const homeMetro = preferences?.home_metro ?? null
  const areaLabel = useHomeMetroLabel(homeMetro)

  // Fails closed: the area half is omitted rather than guessed while the read
  // is in flight. A cached payload still counts as known, because a failed
  // BACKGROUND refetch leaves `data` intact and only flips status, and
  // throwing away a good answer we already hold is a worse lie than showing
  // it. Only a failure with nothing cached is worth reporting.
  const areaKnown = hasScopeAxis && Boolean(preferences)
  const readFailed = isError && !preferences

  return (
    <div className="mb-4 border border-border bg-card px-3.5 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-[13px]">
        {areaKnown && (
          <>
            <span className="text-muted-foreground">
              New follows start at:{' '}
              <span className="text-foreground">
                {homeMetro ? 'Near me' : 'Everywhere'}
              </span>
            </span>

            <span className="text-muted-foreground/40" aria-hidden>
              ·
            </span>

            {isEditingArea ? (
              <HomeMetroSelect
                metro={homeMetro}
                ariaLabel="Your area"
                onSaved={() => setIsEditingArea(false)}
              />
            ) : (
              <>
                <span className="text-muted-foreground">
                  Your area:{' '}
                  <span className="text-foreground">
                    {areaLabel ?? 'not set yet'}
                  </span>
                </span>
                <BracketLink
                  label="change"
                  ariaLabel="Change your area"
                  onClick={() => setIsEditingArea(true)}
                  className="font-mono text-[11px]"
                />
              </>
            )}
          </>
        )}

        {/* FAILED is not PENDING, the rule the entity-page twin states and
            obeys. Without this the tab degrades silently on a failed read: the
            bar loses its area half AND every row's bracket disappears, because
            an unknown home area makes each menu render null. No message, no
            retry, and a user reasonably concludes their follows carry no alert
            subscription at all. Two controls over one field must not disagree
            about whether a failure is worth mentioning. */}
        {readFailed && (
          <>
            <span className="text-muted-foreground" role="alert">
              Couldn&apos;t load your alert settings.
            </span>
            <BracketLink
              label="retry"
              onClick={() => void refetch()}
              className="font-mono text-[11px]"
            />
          </>
        )}

        <span className="grow" />

        <BracketLink
          label="custom alerts →"
          href={CUSTOM_ALERTS_HREF}
          className="font-mono text-[11px]"
        />
      </div>

      {/* Only where there is something pending to disclose. Artist show
          alerts deliver (PSY-1896), so an "any day now" line above the
          Artists tab would be the opposite kind of lie. */}
      {pendingNote && (
        <p className="mt-2 text-xs text-muted-foreground">{pendingNote}</p>
      )}
    </div>
  )
}
