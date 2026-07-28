import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { useWebAuthnSupport } from './useWebAuthnSupport'

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
}))

const supportsMock = vi.mocked(browserSupportsWebAuthn)

// Stand-in for the passkey components: renders one thing when WebAuthn is
// supported and another when it isn't, which is the shape that produced the
// mismatch.
function Probe() {
  return useWebAuthnSupport() ? <button>passkey</button> : <span>none</span>
}

describe('useWebAuthnSupport', () => {
  beforeEach(() => {
    supportsMock.mockReset()
    supportsMock.mockReturnValue(true)
  })

  it('reports the real capability in the browser', () => {
    supportsMock.mockReturnValue(true)
    expect(renderHook(() => useWebAuthnSupport()).result.current).toBe(true)

    supportsMock.mockReturnValue(false)
    expect(renderHook(() => useWebAuthnSupport()).result.current).toBe(false)
  })

  // The regression this hook exists to prevent: `browserSupportsWebAuthn()`
  // returns false on the server and true in the browser, so calling it
  // directly during render made the server emit a tree the client immediately
  // contradicted. React discarded the SSR tree on /auth and the login form was
  // rebuilt underneath whatever had already been typed into it.
  it('renders the supported tree on the server even when the capability read says otherwise', () => {
    supportsMock.mockReturnValue(false)
    expect(renderToString(<Probe />)).toContain('passkey')
  })

  it('agrees with the client render for a browser that supports WebAuthn', () => {
    supportsMock.mockReturnValue(false)
    const server = renderToString(<Probe />)
    supportsMock.mockReturnValue(true)
    const client = renderToString(<Probe />)
    expect(server).toBe(client)
  })
})
