import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BottomTabBar } from './BottomTabBar'
import { CookieConsentBanner } from '../CookieConsentBanner'
import {
  CONSENT_VERSION,
  CookieConsentProvider,
  STORAGE_KEY,
} from '@/lib/context/CookieConsentContext'
import { makeAuthFixture } from '@/test/authFixture'

// The Account sheet's Cookie preferences row against the real consent
// provider and the real preferences dialog that CookieConsentBanner mounts,
// for a visitor who has already saved a decision (so the banner, the other
// way into the dialog, is gone).

vi.mock('next/navigation', () => ({
  usePathname: () => '/atlas',
}))
vi.mock('next/link', () => import('@/test/mocks/nextLink'))

const authFixture = makeAuthFixture<{ email: string; is_admin: boolean }>(vi.fn())
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () =>
    authFixture({
      user: { email: 'user@test.com', is_admin: false },
      authStatus: 'authenticated',
    }),
}))
vi.mock('@/features/notifications/hooks', () => ({
  useUnreadNotificationCount: () => 0,
}))
vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: 'dark', setTheme: vi.fn() }),
}))

function saveDecision(analytics: boolean) {
  const now = Date.now()
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({
      version: CONSENT_VERSION,
      timestamp: new Date(now).toISOString(),
      expiresAt: new Date(now + 86_400_000).toISOString(),
      gpcDetected: false,
      categories: { essential: true, analytics },
      consentMethod: 'banner_reject_all',
    })
  )
}

function renderChrome() {
  return render(
    <CookieConsentProvider>
      <BottomTabBar />
      <CookieConsentBanner />
    </CookieConsentProvider>
  )
}

describe('Account sheet Cookie preferences row', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('opens the preferences dialog over a closed sheet once a decision is saved', async () => {
    saveDecision(false)
    const user = userEvent.setup()
    renderChrome()
    expect(screen.queryByRole('dialog', { name: 'Cookie consent' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Account' }))
    const sheet = await screen.findByRole('dialog', { name: 'Account' })
    await user.click(within(sheet).getByRole('button', { name: 'Cookie preferences' }))

    const dialog = await screen.findByRole('dialog', { name: 'Cookie Preferences' })
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
    )
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement))
    // The dialog shows the saved decision, and saving from it replaces it.
    const analytics = within(dialog).getByRole('switch', { name: 'Analytics cookies' })
    expect(analytics).not.toBeChecked()
    await user.click(analytics)
    await user.click(within(dialog).getByRole('button', { name: 'Save Preferences' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Cookie Preferences' })).not.toBeInTheDocument()
    )
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!).categories.analytics).toBe(true)
  })
})
