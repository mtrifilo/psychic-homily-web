import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import {
  markAccountCreatedThisSession,
  resetAccountCreatedMarkerForTests,
  useAccountCreatedThisSession,
} from './accountCreatedMarker'

function Probe({ userId }: { userId: string }) {
  return <p>{useAccountCreatedThisSession(userId) ? 'new' : 'returning'}</p>
}

beforeEach(() => {
  window.sessionStorage.clear()
  resetAccountCreatedMarkerForTests()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the account-created marker', () => {
  it('answers false on the server, which cannot read the tab', () => {
    markAccountCreatedThisSession(42)

    expect(renderToString(<Probe userId="42" />)).toContain('returning')
  })

  it('names the account a later sign-up in the same page created', () => {
    const { unmount } = render(<Probe userId="42" />)
    expect(screen.getByText('returning')).toBeInTheDocument()
    unmount()

    // A viewer who signs out and creates an account without a reload.
    markAccountCreatedThisSession(43)
    render(<Probe userId="43" />)
    expect(screen.getByText('new')).toBeInTheDocument()
  })

  it('records nothing for an absent id', () => {
    markAccountCreatedThisSession('')

    expect(window.sessionStorage.length).toBe(0)
  })

  it('reads unavailable storage as no marker', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    render(<Probe userId="42" />)

    expect(screen.getByText('returning')).toBeInTheDocument()
  })

  it('survives storage that refuses the write', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })

    expect(() => markAccountCreatedThisSession(42)).not.toThrow()
  })
})
