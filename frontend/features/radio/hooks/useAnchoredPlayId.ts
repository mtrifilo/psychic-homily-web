'use client'

import { useSyncExternalStore } from 'react'
import { parsePlayAnchor } from '../lib/playAnchor'

function subscribe(onChange: () => void): () => void {
  window.addEventListener('hashchange', onChange)
  window.addEventListener('popstate', onChange)
  return () => {
    window.removeEventListener('hashchange', onChange)
    window.removeEventListener('popstate', onChange)
  }
}

function getSnapshot(): number | null {
  return parsePlayAnchor(window.location.hash)
}

function getServerSnapshot(): number | null {
  return null
}

/**
 * The play id the URL hash anchors (`#play-{id}`), or null.
 *
 * App Router client navigations move the URL with pushState, which fires no
 * event, so the hash is a snapshot re-read on every render of the caller as
 * well as on hashchange and popstate. A navigation to another episode date
 * re-renders the caller with a hash-less URL, which is what clears the
 * anchor. Null on the server: the hash never reaches it.
 */
export function useAnchoredPlayId(): number | null {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
