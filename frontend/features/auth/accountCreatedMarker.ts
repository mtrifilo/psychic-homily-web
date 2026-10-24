'use client'

import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { isSameUserId, type UserIdLike } from './authUser'

/**
 * Names the account a sign-up in this tab created, so the signed-in home can
 * greet it as new for the rest of the page load. Written by every sign-up path
 * at the moment it completes its claim; read once, by the home's saved-shows
 * module, and forgotten when the session ends.
 *
 * sessionStorage, not a backend field: "created in this session" is a fact
 * about this tab, and the sign-up responses do not carry it. The stored value
 * is the account's id, so a different account signing in on the same tab never
 * matches it.
 */
const STORAGE_KEY = 'ph-account-created-this-session'

/**
 * The id the first read took out of storage. `undefined` means storage has not
 * been read since the page loaded or since the last sign-up wrote it; `null`
 * means it was read and held nothing. Held here because the read clears the
 * key, and every later render must see the same answer the first one did.
 */
let consumedId: string | null | undefined

/**
 * Records that a sign-up in this tab created `userId`. Storage that cannot be
 * written leaves the next home greeting the account as returning, which is the
 * only cost.
 */
export function markAccountCreatedThisSession(userId: UserIdLike): void {
  if (!userId) return
  // The next read goes back to storage even if an earlier one already ran in
  // this page, which happens when a viewer signs out and creates an account
  // without a reload.
  consumedId = undefined
  try {
    window.sessionStorage.setItem(STORAGE_KEY, String(userId))
  } catch {
    // Storage unavailable: nothing to record.
  }
}

/** Takes the id out of storage on the first call; later calls repeat it. */
function readCreatedIdOnce(): string | null {
  if (consumedId === undefined) {
    try {
      consumedId = window.sessionStorage.getItem(STORAGE_KEY)
      window.sessionStorage.removeItem(STORAGE_KEY)
    } catch {
      consumedId = null
    }
  }
  return consumedId
}

/**
 * Whether `userId` is the account a sign-up in this tab created, for the rest
 * of this page load.
 *
 * The server cannot see sessionStorage, so its render and the hydration render
 * answer false and a full page load corrects to true one commit later. A
 * client-side navigation from the sign-up answers true on its first render.
 */
export function useAccountCreatedThisSession(userId: UserIdLike): boolean {
  return useHydrated() && isSameUserId(readCreatedIdOnce(), userId)
}

/**
 * Forgets the marker, stored or already read, so a later sign-in on this page
 * is greeted as returning. Called when a session ends.
 */
export function forgetAccountCreatedThisSession(): void {
  consumedId = undefined
  try {
    window.sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage unavailable: nothing stored to remove.
  }
}
