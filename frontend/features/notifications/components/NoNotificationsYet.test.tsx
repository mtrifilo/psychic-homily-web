import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NoNotificationsYet } from './NoNotificationsYet'

const DESTINATIONS = [
  ['artists', '/artists'],
  ['venues', '/venues'],
  ['scenes', '/scenes'],
] as const

describe('NoNotificationsYet', () => {
  describe('page variant', () => {
    it('explains what arrives here in the board copy', () => {
      render(<NoNotificationsYet variant="page" />)

      expect(screen.getByText('Nothing has arrived yet.')).toBeInTheDocument()
      expect(
        screen.getByText(
          'New shows from the artists, venues and scenes you follow land here, along with replies to your comments and mentions.'
        )
      ).toBeInTheDocument()
      expect(screen.getByText('Find something to follow:')).toBeInTheDocument()
    })

    it.each(DESTINATIONS)('links [%s] to %s', (label, href) => {
      render(<NoNotificationsYet variant="page" />)

      expect(screen.getByRole('link', { name: label })).toHaveAttribute(
        'href',
        href
      )
    })

    it('is the default variant', () => {
      render(<NoNotificationsYet />)
      expect(screen.getByText('Nothing has arrived yet.')).toBeInTheDocument()
    })
  })

  describe('popover variant', () => {
    it('renders the terse copy without the page heading', () => {
      render(<NoNotificationsYet variant="popover" />)

      expect(
        screen.getByText(
          'Nothing yet. New shows from artists, venues and scenes you follow land here.'
        )
      ).toBeInTheDocument()
      expect(
        screen.queryByText('Nothing has arrived yet.')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByText('Find something to follow:')
      ).not.toBeInTheDocument()
    })

    it.each(DESTINATIONS)('links [%s] to %s', (label, href) => {
      render(<NoNotificationsYet variant="popover" />)

      expect(screen.getByRole('link', { name: label })).toHaveAttribute(
        'href',
        href
      )
    })
  })

  it('fires onNavigate when a destination link is clicked', async () => {
    const onNavigate = vi.fn()
    const user = userEvent.setup()
    render(<NoNotificationsYet variant="popover" onNavigate={onNavigate} />)

    await user.click(screen.getByRole('link', { name: 'venues' }))

    expect(onNavigate).toHaveBeenCalledTimes(1)
  })

  it.each(['page', 'popover'] as const)(
    'carries no em dash in the %s copy',
    variant => {
      const { container } = render(<NoNotificationsYet variant={variant} />)
      expect(container.textContent).not.toContain('\u2014')
    }
  )
})
