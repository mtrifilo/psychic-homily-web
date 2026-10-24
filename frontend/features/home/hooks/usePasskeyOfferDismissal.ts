'use client'

import { useCallback, useState, useSyncExternalStore } from 'react'

/** One key per account, so a second account on this browser is still offered. */
const keyFor = (userId: string) => `ph-passkey-offer-dismissed:${userId}`

// The 'storage' event reaches only OTHER tabs; this one tells the tab that
// wrote the dismissal.
const SAME_TAB_EVENT = 'ph-passkey-offer-dismissed'

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange)
  window.addEventListener(SAME_TAB_EVENT, onChange)
  return () => {
    window.removeEventListener('storage', onChange)
    window.removeEventListener(SAME_TAB_EVENT, onChange)
  }
}

/**
 * Whether the passkey offer was dismissed for `userId` on this browser, and the
 * dismissal itself. A dismissal is final: nothing here ever clears it.
 *
 * The server and the hydration render answer DISMISSED, because the server
 * cannot read localStorage and a card painted in the server HTML would flash
 * and collapse on every visit after the viewer dismissed it. The card appears
 * one commit after hydration for a viewer who has not.
 *
 * Unavailable storage (a private window, blocked site data) reads as not
 * dismissed, so the card can return on a later visit there. That is
 * acceptable: the cost is one more offer, never a lost passkey. Within the
 * page, a dismissal that could not be stored still hides the card.
 */
export function usePasskeyOfferDismissal(userId: string): {
  dismissed: boolean
  dismiss: () => void
} {
  // Tagged with the account it was made for, so it never hides the card for
  // another account that signs in on this page.
  const [dismissedInPage, setDismissedInPage] = useState<string | null>(null)

  const getSnapshot = useCallback(() => {
    if (!userId) return true
    try {
      return window.localStorage.getItem(keyFor(userId)) !== null
    } catch {
      return false
    }
  }, [userId])

  const stored = useSyncExternalStore(subscribe, getSnapshot, () => true)

  const dismiss = useCallback(() => {
    setDismissedInPage(userId)
    try {
      window.localStorage.setItem(keyFor(userId), new Date().toISOString())
      window.dispatchEvent(new Event(SAME_TAB_EVENT))
    } catch {
      // Storage unavailable: the in-page flag above still hides the card.
    }
  }, [userId])

  return { dismissed: stored || dismissedInPage === userId, dismiss }
}
