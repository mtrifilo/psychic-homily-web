import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Footer from './Footer'

// --- Mocks ---

const mockOpenPreferences = vi.fn()

vi.mock('@/lib/context/CookieConsentContext', () => ({
  useCookieConsent: () => ({
    openPreferences: mockOpenPreferences,
  }),
}))

vi.mock('next/link', () => ({
  default: ({ children, href, ...props }: { children: React.ReactNode; href: string; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

describe('Footer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders copyright with current year', () => {
    render(<Footer />)
    const currentYear = new Date().getFullYear()
    expect(screen.getByText(`\u00A9 ${currentYear} Psychic Homily`)).toBeInTheDocument()
  })

  it('renders footer element', () => {
    render(<Footer />)
    const footer = document.querySelector('footer')
    expect(footer).toBeInTheDocument()
  })

  it('renders Privacy Policy link', () => {
    render(<Footer />)
    const link = screen.getByText('Privacy Policy')
    expect(link).toBeInTheDocument()
    expect(link.closest('a')).toHaveAttribute('href', '/privacy')
  })

  it('renders Terms of Service link', () => {
    render(<Footer />)
    const link = screen.getByText('Terms of Service')
    expect(link).toBeInTheDocument()
    expect(link.closest('a')).toHaveAttribute('href', '/terms')
  })

  it('renders Contact link with mailto', () => {
    render(<Footer />)
    const link = screen.getByText('Contact')
    expect(link).toBeInTheDocument()
    expect(link.closest('a')).toHaveAttribute('href', 'mailto:hello@psychichomily.com')
  })

  it('renders Cookie Preferences button', () => {
    render(<Footer />)
    expect(screen.getByText('Cookie Preferences')).toBeInTheDocument()
  })

  it('Cookie Preferences button is a button element (not a link)', () => {
    render(<Footer />)
    const button = screen.getByText('Cookie Preferences')
    expect(button.tagName).toBe('BUTTON')
    expect(button).toHaveAttribute('type', 'button')
  })

  it('calls openPreferences when Cookie Preferences is clicked', async () => {
    const user = userEvent.setup()
    render(<Footer />)

    await user.click(screen.getByText('Cookie Preferences'))
    expect(mockOpenPreferences).toHaveBeenCalledOnce()
  })

  it('has nav element for navigation links', () => {
    render(<Footer />)
    const nav = document.querySelector('nav')
    expect(nav).toBeInTheDocument()
  })

  it('renders all four nav items', () => {
    render(<Footer />)
    const nav = document.querySelector('nav')
    // 3 links + 1 button
    const links = nav?.querySelectorAll('a')
    const buttons = nav?.querySelectorAll('button')
    expect(links?.length).toBe(3)
    expect(buttons?.length).toBe(1)
  })

  it('Cookie Preferences button click fires openPreferences exactly once and does not navigate', async () => {
    const user = userEvent.setup()
    render(<Footer />)

    await user.click(screen.getByText('Cookie Preferences'))
    expect(mockOpenPreferences).toHaveBeenCalledOnce()

    // Double-click should fire twice (sanity check the binding is direct,
    // not throttled, since the cookie dialog needs to remount cleanly).
    await user.click(screen.getByText('Cookie Preferences'))
    expect(mockOpenPreferences).toHaveBeenCalledTimes(2)
  })

  it('uses the same href for Privacy and Cookie Preferences contexts (privacy page)', () => {
    render(<Footer />)
    // Privacy link is the long-form policy page; the cookie preferences
    // dialog is a separate trigger. Pin that they're independent UX paths.
    const privacy = screen.getByText('Privacy Policy').closest('a')!
    const cookie = screen.getByText('Cookie Preferences')
    expect(privacy.getAttribute('href')).toBe('/privacy')
    expect(cookie.tagName).toBe('BUTTON') // NOT a link to /privacy
  })

  it('keeps Privacy Policy and Terms of Service links in tab order before Contact', () => {
    render(<Footer />)
    const nav = document.querySelector('nav')!
    const linkTexts = Array.from(nav.querySelectorAll('a')).map(a => a.textContent)
    // The legal pair must come first so screen-reader / keyboard users hit
    // them before the mailto. If a future refactor reorders these silently,
    // this catches it.
    expect(linkTexts).toEqual(['Privacy Policy', 'Terms of Service', 'Contact'])
  })

  it('renders the year as a 4-digit number, not NaN or 0', () => {
    render(<Footer />)
    const text = screen.getByText(/Psychic Homily/).textContent ?? ''
    // Pin the format: 4-digit year, NOT "NaN" or "0" (defensive against
    // a regression that pre-computes the year at module load with a
    // broken Date constructor).
    expect(text).toMatch(/©\s+\d{4}\s+Psychic Homily/)
    expect(text).not.toMatch(/NaN/)
    expect(text).not.toMatch(/©\s+0\s+/)
  })
})
