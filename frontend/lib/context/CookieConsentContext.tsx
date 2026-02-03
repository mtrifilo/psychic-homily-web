'use client'

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useMemo,
  ReactNode,
} from 'react'

const STORAGE_KEY = 'cookie-consent'
const CONSENT_VERSION = 1
const CONSENT_DURATION_MONTHS = 6

export interface CookieConsentState {
  version: number
  timestamp: string
  expiresAt: string
  gpcDetected: boolean
  categories: {
    essential: true
    analytics: boolean
  }
  consentMethod: 'banner_accept_all' | 'banner_reject_all' | 'customized'
}

interface CookieConsentContextType {
  consent: CookieConsentState | null
  isLoaded: boolean
  showBanner: boolean
  canUseAnalytics: boolean
  gpcSignalDetected: boolean
  preferencesOpen: boolean
  acceptAll: () => void
  rejectAll: () => void
  savePreferences: (analytics: boolean) => void
  openPreferences: () => void
  closePreferences: () => void
}

const CookieConsentContext = createContext<CookieConsentContextType | undefined>(
  undefined
)

function detectGPCSignal(): boolean {
  if (typeof window === 'undefined') return false

  // Check for Global Privacy Control signal
  const nav = navigator as Navigator & {
    globalPrivacyControl?: boolean
    doNotTrack?: string
  }

  if (nav.globalPrivacyControl === true) {
    return true
  }

  // Also check legacy Do Not Track as a fallback indicator
  if (nav.doNotTrack === '1') {
    return true
  }

  return false
}

function isConsentExpired(consent: CookieConsentState): boolean {
  const expiresAt = new Date(consent.expiresAt)
  return new Date() > expiresAt
}

function createExpirationDate(): string {
  const date = new Date()
  date.setMonth(date.getMonth() + CONSENT_DURATION_MONTHS)
  return date.toISOString()
}

function loadConsent(): CookieConsentState | null {
  if (typeof window === 'undefined') return null

  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (!stored) return null

    const consent = JSON.parse(stored) as CookieConsentState

    // Check version compatibility
    if (consent.version !== CONSENT_VERSION) {
      localStorage.removeItem(STORAGE_KEY)
      return null
    }

    // Check expiration
    if (isConsentExpired(consent)) {
      localStorage.removeItem(STORAGE_KEY)
      return null
    }

    return consent
  } catch {
    localStorage.removeItem(STORAGE_KEY)
    return null
  }
}

function saveConsent(consent: CookieConsentState): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(STORAGE_KEY, JSON.stringify(consent))
}

interface CookieConsentProviderProps {
  children: ReactNode
}

export function CookieConsentProvider({ children }: CookieConsentProviderProps) {
  const [consent, setConsent] = useState<CookieConsentState | null>(null)
  const [isLoaded, setIsLoaded] = useState(false)
  const [gpcSignalDetected, setGpcSignalDetected] = useState(false)
  const [preferencesOpen, setPreferencesOpen] = useState(false)

  // Load consent and detect GPC on mount
  useEffect(() => {
    const storedConsent = loadConsent()
    const gpcDetected = detectGPCSignal()

    setConsent(storedConsent)
    setGpcSignalDetected(gpcDetected)
    setIsLoaded(true)
  }, [])

  const showBanner = useMemo(() => {
    return isLoaded && consent === null
  }, [isLoaded, consent])

  const canUseAnalytics = useMemo(() => {
    if (!consent) return false
    return consent.categories.analytics === true
  }, [consent])

  const acceptAll = useCallback(() => {
    const newConsent: CookieConsentState = {
      version: CONSENT_VERSION,
      timestamp: new Date().toISOString(),
      expiresAt: createExpirationDate(),
      gpcDetected: gpcSignalDetected,
      categories: {
        essential: true,
        analytics: true,
      },
      consentMethod: 'banner_accept_all',
    }
    saveConsent(newConsent)
    setConsent(newConsent)
  }, [gpcSignalDetected])

  const rejectAll = useCallback(() => {
    const newConsent: CookieConsentState = {
      version: CONSENT_VERSION,
      timestamp: new Date().toISOString(),
      expiresAt: createExpirationDate(),
      gpcDetected: gpcSignalDetected,
      categories: {
        essential: true,
        analytics: false,
      },
      consentMethod: 'banner_reject_all',
    }
    saveConsent(newConsent)
    setConsent(newConsent)
  }, [gpcSignalDetected])

  const savePreferences = useCallback(
    (analytics: boolean) => {
      const newConsent: CookieConsentState = {
        version: CONSENT_VERSION,
        timestamp: new Date().toISOString(),
        expiresAt: createExpirationDate(),
        gpcDetected: gpcSignalDetected,
        categories: {
          essential: true,
          analytics,
        },
        consentMethod: 'customized',
      }
      saveConsent(newConsent)
      setConsent(newConsent)
      setPreferencesOpen(false)
    },
    [gpcSignalDetected]
  )

  const openPreferences = useCallback(() => {
    setPreferencesOpen(true)
  }, [])

  const closePreferences = useCallback(() => {
    setPreferencesOpen(false)
  }, [])

  const value: CookieConsentContextType = useMemo(
    () => ({
      consent,
      isLoaded,
      showBanner,
      canUseAnalytics,
      gpcSignalDetected,
      preferencesOpen,
      acceptAll,
      rejectAll,
      savePreferences,
      openPreferences,
      closePreferences,
    }),
    [
      consent,
      isLoaded,
      showBanner,
      canUseAnalytics,
      gpcSignalDetected,
      preferencesOpen,
      acceptAll,
      rejectAll,
      savePreferences,
      openPreferences,
      closePreferences,
    ]
  )

  return (
    <CookieConsentContext.Provider value={value}>
      {children}
    </CookieConsentContext.Provider>
  )
}

export function useCookieConsent() {
  const context = useContext(CookieConsentContext)
  if (context === undefined) {
    throw new Error(
      'useCookieConsent must be used within a CookieConsentProvider'
    )
  }
  return context
}
