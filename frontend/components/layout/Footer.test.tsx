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
})
