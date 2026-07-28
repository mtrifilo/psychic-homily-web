'use client'

import { useSyncExternalStore } from 'react'
import { browserSupportsWebAuthn } from '@simplewebauthn/browser'

// WebAuthn support is a property of the document's browser and origin: it
// cannot change while the page is open, so there is nothing to subscribe to.
// Must be module-level — a new closure per render would make React treat the
// store as changed on every render.
const subscribe = () => () => {}

const getSnapshot = () => browserSupportsWebAuthn()

// The server cannot know whether the viewer's browser can do WebAuthn, so it
// answers with the case that is true for every browser released since 2019.
// React reuses this same value for the client's hydration render and only then
// re-renders with the real capability, which is what makes the server HTML and
// the client's first render identical *by construction* rather than by
// suppressing the warning: calling `browserSupportsWebAuthn()` directly during
// render returns `false` on the server and `true` in the browser, so any
// component that gates its output on it produces two different trees.
//
// Optimistic rather than pessimistic on purpose. `false` would agree just as
// well, but it would strip the passkey button out of the server HTML for
// everyone and pop it back in after hydration — re-introducing exactly the
// "first paint is not the real tree" problem the surrounding change exists to
// fix. This way the common path renders once and never changes; only a browser
// that genuinely lacks WebAuthn drops the passkey UI, one commit after
// hydration.
const getServerSnapshot = () => true

/**
 * Whether this browser can perform WebAuthn (passkey) ceremonies, in a form
 * that is safe to branch the rendered tree on during SSR.
 *
 * Use this instead of calling `browserSupportsWebAuthn()` in a component body
 * whenever the result decides what gets rendered.
 */
export function useWebAuthnSupport(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}
