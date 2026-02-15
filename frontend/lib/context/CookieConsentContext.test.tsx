import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { ReactNode } from 'react'
import {
  CookieConsentProvider,
  useCookieConsent,
  loadConsent,
  saveConsent,
  isConsentExpired,
  createExpirationDate,
  detectGPCSignal,
  STORAGE_KEY,
  CONSENT_VERSION,
  type CookieConsentState,
} from './CookieConsentContext'

// --- localStorage mock helpers ---

function setStoredConsent(consent: CookieConsentState) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(consent))
}

function makeValidConsent(overrides?: Partial<CookieConsentState>): CookieConsentState {
  const future = new Date()
  future.setMonth(future.getMonth() + 3)
  return {
    version: CONSENT_VERSION,
    timestamp: new Date().toISOString(),
    expiresAt: future.toISOString(),
    gpcDetected: false,
    categories: { essential: true, analytics: true },
    consentMethod: 'banner_accept_all',
    ...overrides,
  }
}

// --- Pure function tests ---

describe('loadConsent', () => {
  beforeEach(() => localStorage.clear())

  it('returns null when no stored consent', () => {
    expect(loadConsent()).toBeNull()
  })

  it('returns parsed consent from localStorage', () => {
    const consent = makeValidConsent()
    setStoredConsent(consent)
    expect(loadConsent()).toEqual(consent)
  })

  it('returns null and clears storage for expired consent', () => {
    const past = new Date()
    past.setMonth(past.getMonth() - 1)
    const consent = makeValidConsent({ expiresAt: past.toISOString() })
    setStoredConsent(consent)

    expect(loadConsent()).toBeNull()
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
  })

  it('returns null and clears storage for wrong version', () => {
    const consent = makeValidConsent({ version: 999 })
    setStoredConsent(consent)

    expect(loadConsent()).toBeNull()
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
  })

  it('returns null and clears storage for invalid JSON', () => {
    localStorage.setItem(STORAGE_KEY, '{invalid json!!!')

    expect(loadConsent()).toBeNull()
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
  })
})

describe('saveConsent', () => {
  beforeEach(() => localStorage.clear())

  it('persists consent to localStorage', () => {
    const consent = makeValidConsent()
    saveConsent(consent)
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)).toEqual(consent)
  })
})

describe('isConsentExpired', () => {
  it('returns false for a future expiration date', () => {
    const future = new Date()
    future.setMonth(future.getMonth() + 3)
    const consent = makeValidConsent({ expiresAt: future.toISOString() })
    expect(isConsentExpired(consent)).toBe(false)
  })

  it('returns true for a past expiration date', () => {
    const past = new Date()
    past.setMonth(past.getMonth() - 1)
    const consent = makeValidConsent({ expiresAt: past.toISOString() })
    expect(isConsentExpired(consent)).toBe(true)
  })
})

describe('createExpirationDate', () => {
  it('returns a date approximately 6 months in the future', () => {
    const result = new Date(createExpirationDate())
    const now = new Date()
    const diffMs = result.getTime() - now.getTime()
    const diffDays = diffMs / (1000 * 60 * 60 * 24)
    // 6 months is roughly 180 days, give generous range
    expect(diffDays).toBeGreaterThan(150)
    expect(diffDays).toBeLessThan(200)
  })
})

describe('detectGPCSignal', () => {
  const originalNavigator = window.navigator

  afterEach(() => {
    // Reset navigator overrides
    Object.defineProperty(window, 'navigator', {
      value: originalNavigator,
      writable: true,
      configurable: true,
    })
  })

  it('returns true when globalPrivacyControl is true', () => {
    Object.defineProperty(window, 'navigator', {
      value: { ...originalNavigator, globalPrivacyControl: true },
      writable: true,
      configurable: true,
    })
    expect(detectGPCSignal()).toBe(true)
  })

  it('returns true when doNotTrack is "1"', () => {
    Object.defineProperty(window, 'navigator', {
      value: { ...originalNavigator, doNotTrack: '1', globalPrivacyControl: undefined },
      writable: true,
      configurable: true,
    })
    expect(detectGPCSignal()).toBe(true)
  })

  it('returns false when neither signal is set', () => {
    Object.defineProperty(window, 'navigator', {
      value: { ...originalNavigator, globalPrivacyControl: undefined, doNotTrack: undefined },
      writable: true,
      configurable: true,
    })
    expect(detectGPCSignal()).toBe(false)
  })

  it('returns false when doNotTrack is "0"', () => {
    Object.defineProperty(window, 'navigator', {
      value: { ...originalNavigator, globalPrivacyControl: false, doNotTrack: '0' },
      writable: true,
      configurable: true,
    })
    expect(detectGPCSignal()).toBe(false)
  })
})

// --- Provider integration tests ---

function wrapper({ children }: { children: ReactNode }) {
  return <CookieConsentProvider>{children}</CookieConsentProvider>
}

describe('CookieConsentProvider', () => {
  beforeEach(() => localStorage.clear())

  it('shows banner when no consent stored', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    // Wait for useEffect to run
    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    expect(result.current.showBanner).toBe(true)
    expect(result.current.consent).toBeNull()
  })

  it('hides banner after acceptAll', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    act(() => result.current.acceptAll())

    expect(result.current.showBanner).toBe(false)
    expect(result.current.consent).not.toBeNull()
  })

  it('canUseAnalytics is true after acceptAll', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    act(() => result.current.acceptAll())

    expect(result.current.canUseAnalytics).toBe(true)
  })

  it('canUseAnalytics is false after rejectAll', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    act(() => result.current.rejectAll())

    expect(result.current.canUseAnalytics).toBe(false)
  })

  it('savePreferences(false) disables analytics', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    act(() => result.current.savePreferences(false))

    expect(result.current.canUseAnalytics).toBe(false)
    expect(result.current.consent?.consentMethod).toBe('customized')
  })

  it('savePreferences(true) enables analytics', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    act(() => result.current.savePreferences(true))

    expect(result.current.canUseAnalytics).toBe(true)
    expect(result.current.consent?.consentMethod).toBe('customized')
  })

  it('persists consent to localStorage after acceptAll', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    act(() => result.current.acceptAll())

    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!)
    expect(stored.categories.analytics).toBe(true)
    expect(stored.consentMethod).toBe('banner_accept_all')
    expect(stored.version).toBe(CONSENT_VERSION)
  })

  it('loads existing consent from localStorage', async () => {
    const consent = makeValidConsent()
    setStoredConsent(consent)

    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    expect(result.current.showBanner).toBe(false)
    expect(result.current.canUseAnalytics).toBe(true)
  })

  it('openPreferences / closePreferences toggles state', async () => {
    const { result } = renderHook(() => useCookieConsent(), { wrapper })

    await vi.waitFor(() => {
      expect(result.current.isLoaded).toBe(true)
    })

    expect(result.current.preferencesOpen).toBe(false)

    act(() => result.current.openPreferences())
    expect(result.current.preferencesOpen).toBe(true)

    act(() => result.current.closePreferences())
    expect(result.current.preferencesOpen).toBe(false)
  })

  it('throws when useCookieConsent is used outside provider', () => {
    // Suppress console.error from React for expected error
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => {
      renderHook(() => useCookieConsent())
    }).toThrow('useCookieConsent must be used within a CookieConsentProvider')
    spy.mockRestore()
  })
})
