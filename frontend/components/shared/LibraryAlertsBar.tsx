'use client'

import { useState } from 'react'
import { BracketLink } from './BracketLink'
import { HomeMetroSelect, useHomeMetroLabel } from './HomeMetroField'
import { useAlertPreferences } from '@/features/auth/hooks/useAlertPreferences'
import {
  ALERTS_HREF,
  ALERTS_PAUSED_SUMMARY,
  CUSTOM_ALERTS_HREF,
  followAlertHasChannel,
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

  // Fetched on EVERY tab this bar renders on, not only the scoped ones.
  //
  // `alert_defaults` carries one `shows` key covering artist and venue show
  // alerts alike, so the pause is exactly as real on the Venues tab. Gating
  // the read on the scope axis meant a Venues tab could show a column of
  // paused brackets under a bar that never mentioned it, while the Artists tab
  // one click away explained it and offered the way out.
  //
  // It costs one request per five minutes per session, shared with the entity
  // pages and the settings card, and only on the tabs whose follows carry a
  // subscription at all. On the Venues tab this bar is the only observer: the
  // page's own `useHomeMetroState` is deliberately disabled there.
  const {
    data: preferences,
    isError,
    refetch,
  } = useAlertPreferences()
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

  // Whether a follow made RIGHT NOW would arrive silent.
  //
  // Read from the ACCOUNT matrix, not from the rows on screen, because that is
  // the question this half of the bar answers: what a new follow starts at. A
  // new follow inherits the account channels and nothing else, so no sample of
  // existing rows can answer it. Deriving it from the rows instead was wrong
  // three ways: one follow switched off by hand collapsed an `every`, an `any`
  // would have claimed a pause over rows that deliver, and either way the
  // sample is one page of a cursor-paginated list, so the answer changed on
  // Load more.
  //
  // Gated on the PREFERENCES read, not on the scope axis: one `shows` key
  // covers both follow types, so a venue follow pauses on the same fact.
  const accountShowChannels = preferences?.alert_defaults?.shows
  const newFollowsPaused =
    accountShowChannels !== undefined &&
    !followAlertHasChannel(accountShowChannels)

  return (
    <div className="mb-4 border border-border bg-card px-3.5 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-[13px]">
        {/* The pause is stated on every tab this bar renders on, because it
            is true of every follow type here. The starting SCOPE is a scoped
            tab's fact only, and it is the half the pause replaces: with no
            channel on, a reader does not take "New follows start at: Near me"
            as a statement about geography, they take it as a delivery
            promise, directly above a column of brackets saying paused. */}
        {newFollowsPaused ? (
          <>
            {/* Same lead-in as the sentence it replaces, because it answers
                the same question. An unqualified "New-show alerts: paused"
                would assert a state over every row, and a follow carrying a
                per-follow channel override still delivers; this half is only
                ever about what a NEW follow inherits. */}
            <span className="text-muted-foreground">
              New follows start at:{' '}
              <span className="text-foreground">{ALERTS_PAUSED_SUMMARY}</span>
            </span>
            <BracketLink
              label="turn a channel on"
              ariaLabel="New follows start paused. Turn a channel on in alert settings."
              href={ALERTS_HREF}
              className="font-mono text-[11px]"
            />
          </>
        ) : (
          areaKnown && (
            <span className="text-muted-foreground">
              New follows start at:{' '}
              <span className="text-foreground">
                {homeMetro ? 'Near me' : 'Everywhere'}
              </span>
            </span>
          )
        )}

        {/* The AREA half stays under a pause: it is what "near me" will mean
            when a channel comes back, and this bar is the only place on the
            page it can be changed. The separator leads it rather than
            trailing the block above, so a tab with no area half (Venues)
            cannot render an orphaned dot. */}
        {areaKnown && (
          <>
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
            obeys. On a scoped tab the degradation is severe: the bar loses its
            area half AND every row's bracket disappears, because an unknown
            home area makes each menu render null, so a user reasonably
            concludes their follows carry no alert subscription at all. On the
            Venues tab the rows survive (their option list needs no area), but
            whether those alerts are paused is now unknown, which is its own
            thing worth saying. Either way, two controls over one field must
            not disagree about whether a failure is worth mentioning. */}
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
