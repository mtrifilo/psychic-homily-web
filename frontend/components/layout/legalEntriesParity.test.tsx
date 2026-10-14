import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import Footer from './Footer'
import { legalSheetRows } from './nav/navData'

// The site footer's About column and the mobile Account sheet's legal block
// are the two homes of the legal entries. The footer does not render below
// `xl` on some routes (footerVisibility.ts), where the sheet is the only one,
// so every legal destination the footer links must also be a sheet row, and
// both must carry the cookie preferences control. Contact (mailto) and the
// newsletter (an external site) are not legal entries.

vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => ({ openPreferences: vi.fn() }),
}))
vi.mock('next/link', () => import('@/test/mocks/nextLink'))
vi.mock('next/navigation', () => ({
  usePathname: () => '/',
  useRouter: () => ({ prefetch: () => {} }),
}))

describe('legal entries: footer About column and Account sheet', () => {
  it('lists every footer legal link among the sheet rows, and both open cookie preferences', () => {
    render(<Footer />)
    const about = screen.getByRole('navigation', { name: 'About' })
    const footerLegalHrefs = within(about)
      .getAllByRole('link')
      .map(link => link.getAttribute('href')!)
      .filter(href => href.startsWith('/'))
    const sheetHrefs = legalSheetRows.flatMap(row => (row.kind === 'link' ? [row.href] : []))

    expect(footerLegalHrefs.length).toBeGreaterThan(0)
    expect(sheetHrefs).toEqual(expect.arrayContaining(footerLegalHrefs))
    expect(within(about).getByRole('button', { name: /cookie preferences/i })).toBeInTheDocument()
    expect(legalSheetRows.some(row => row.kind === 'cookie-preferences')).toBe(true)
  })
})
