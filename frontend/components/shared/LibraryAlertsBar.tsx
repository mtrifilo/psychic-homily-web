'use client'

import { useState } from 'react'
import { BracketLink } from './BracketLink'
import { HomeMetroSelect, useHomeMetroLabel } from './HomeMetroField'
import { useAlertPreferences } from '@/features/auth/hooks/useAlertPreferences'
import {
  CUSTOM_ALERTS_HREF,
  FOLLOW_ALERTS_PENDING_NOTE,
} from './followAlertChoices'

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
export function LibraryAlertsBar() {
  const { data: preferences, isLoading } = useAlertPreferences()
  const [isEditingArea, setIsEditingArea] = useState(false)
  const homeMetro = preferences?.home_metro ?? null
  const areaLabel = useHomeMetroLabel(homeMetro)

  if (isLoading || !preferences) return null

  return (
    <div className="mb-4 border border-border bg-card px-3.5 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-[13px]">
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

        <span className="grow" />

        <BracketLink
          label="custom alerts →"
          href={CUSTOM_ALERTS_HREF}
          className="font-mono text-[11px]"
        />
      </div>

      <p className="mt-2 text-xs text-muted-foreground">
        {FOLLOW_ALERTS_PENDING_NOTE}
      </p>
    </div>
  )
}
