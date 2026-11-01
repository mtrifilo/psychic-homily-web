'use client'

import { useEffect, useRef, useState } from 'react'
import { useProfile } from '@/features/auth'
import { cn } from '@/lib/utils'
import { ChangePassword } from './change-password'
import { SetPassword } from './set-password'
import { SETTINGS_ANCHOR_SCROLL_MT, useAnchorScroll } from './useAnchorScroll'

/** The Settings password card's fragment. */
export const PASSWORD_SETTINGS_ANCHOR = 'password'

/**
 * The Settings password card's address, where a sign-in asked for by that
 * card returns the reader.
 */
export const PASSWORD_SETTINGS_HREF = `/profile?tab=settings#${PASSWORD_SETTINGS_ANCHOR}`

/**
 * The Settings password slot: Set a password for an account the profile says
 * has none and that has an email address, and Change password otherwise,
 * including while the profile has not said (an absent `has_password` is not
 * false).
 *
 * The confirmation a set raises is held here rather than in the set card,
 * because that card leaves as soon as the set succeeds and Change password
 * shows it instead.
 */
export function PasswordSettings() {
  const { data: profile } = useProfile()
  const [setConfirmation, setSetConfirmation] = useState<string | null>(null)
  const anchorRef = useAnchorScroll(PASSWORD_SETTINGS_ANCHOR)
  const slotRef = useRef<HTMLDivElement | null>(null)

  // The set card leaves with the focused submit button, so focus moves to the
  // slot, where the confirmation now shows.
  useEffect(() => {
    if (setConfirmation !== null) slotRef.current?.focus()
  }, [setConfirmation])

  // A password signs in with the account's address, so an account without
  // one is not offered a password it could never use.
  const offersSet =
    !!profile?.success &&
    profile.has_password === false &&
    !!profile.user?.email &&
    setConfirmation === null

  return (
    <div
      ref={node => {
        slotRef.current = node
        anchorRef(node)
      }}
      id={PASSWORD_SETTINGS_ANCHOR}
      tabIndex={-1}
      className={cn(SETTINGS_ANCHOR_SCROLL_MT, 'focus:outline-none')}
    >
      {offersSet ? (
        <SetPassword
          providers={(profile.user?.oauth_accounts ?? []).map(account => account.provider)}
          passkeyCount={profile.passkey_count ?? 0}
          reauthReturnTo={PASSWORD_SETTINGS_HREF}
          onPasswordSet={setSetConfirmation}
        />
      ) : (
        <ChangePassword notice={setConfirmation} />
      )}
    </div>
  )
}
