import type { ReactElement } from 'react'
import { hydrateRoot, type Root } from 'react-dom/client'
import { act } from '@testing-library/react'
import { vi } from 'vitest'
import type { AuthStatus } from '@/lib/context/AuthContext'

/** A viewer a route's HTML can be served under, as `deriveMockAuthSignals` takes it. */
export type ViewerState = {
  user: { id: string } | null
  authStatus: AuthStatus
}

export const ANONYMOUS_VIEWER: ViewerState = {
  user: null,
  authStatus: 'anonymous',
}

/**
 * Every viewer state the served markup of a Save control can be rendered for,
 * with the accessible name and disabled state SaveButton gives it there.
 */
export const SAVE_CONTROL_VIEWERS: Array<
  [name: string, viewer: ViewerState, accessibleName: string, disabled: boolean]
> = [
  ['anonymous', ANONYMOUS_VIEWER, 'Sign in to save', false],
  ['unsettled', { user: null, authStatus: 'pending' }, 'Save show', true],
  [
    'signed in',
    { user: { id: '1' }, authStatus: 'authenticated' },
    'Save show',
    false,
  ],
]

/**
 * Hydrate `element` over the server markup already in `container`, and return
 * the root plus every hydration error and console error it raised.
 */
export async function hydrateCollectingErrors(
  container: HTMLElement,
  element: ReactElement
): Promise<{ root: Root; errors: unknown[] }> {
  const errors: unknown[] = []
  const consoleError = vi
    .spyOn(console, 'error')
    .mockImplementation((...args) => errors.push(args))
  let root: Root | undefined
  try {
    await act(async () => {
      root = hydrateRoot(container, element, {
        onRecoverableError: error => errors.push(error),
      })
    })
  } finally {
    consoleError.mockRestore()
  }
  return { root: root!, errors }
}
