import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ALERTS_AREA_HREF, ALERTS_HREF } from '@/components/shared/followAlertChoices'
import { HOME_SECTIONS } from '@/features/home/sections'
import { SETTINGS_SECTIONS } from '../sections'
import { SettingsHub } from './SettingsHub'

// The Home page section mounts the real show / hide / reorder list, which reads
// the profile through the concrete hook module. A live query would leave
// TanStack timers behind, so the read is stubbed with a stored-nothing profile.
vi.mock('@/features/auth/hooks/useAuth', async importOriginal => ({
  ...(await importOriginal<object>()),
  useProfile: () => ({
    data: { success: true, user: { id: 7, preferences: { home_layout: null } } },
    isSuccess: true,
    status: 'success',
  }),
}))

const TITLES = SETTINGS_SECTIONS.map(section => section.title)
const ANCHORS = SETTINGS_SECTIONS.map(section => section.anchor)

const scrollIntoView = vi.fn()

function setHash(hash: string) {
  window.history.replaceState(null, '', `/settings${hash}`)
}

describe('SettingsHub', () => {
  beforeEach(() => {
    scrollIntoView.mockReset()
    Element.prototype.scrollIntoView = scrollIntoView
    setHash('')
  })

  afterEach(() => {
    setHash('')
  })

  it('lists every section on the rail, in order, as fragment links', () => {
    renderWithProviders(<SettingsHub />)

    const rail = screen.getByRole('navigation', { name: 'Settings sections' })
    const links = within(rail).getAllByRole('link')
    expect(links.map(link => link.getAttribute('href'))).toEqual(
      ANCHORS.map(anchor => `#${anchor}`)
    )
    TITLES.forEach((title, index) => {
      expect(links[index]).toHaveTextContent(title)
    })
  })

  it('lists the same sections in the mobile jump index', () => {
    renderWithProviders(<SettingsHub />)

    const index = screen.getByRole('navigation', { name: 'On this page' })
    const links = within(index).getAllByRole('link')
    expect(links.map(link => link.getAttribute('href'))).toEqual(
      ANCHORS.map(anchor => `#${anchor}`)
    )
  })

  it('gives each rail entry its derived count, spoken as settings', () => {
    renderWithProviders(<SettingsHub />)

    const rail = screen.getByRole('navigation', { name: 'Settings sections' })
    const account = within(rail).getByRole('link', { name: /^Account/ })
    expect(account).toHaveTextContent('Account6(6 settings)')
    const appearance = within(rail).getByRole('link', { name: /^Appearance/ })
    expect(appearance).toHaveTextContent('Appearance1(1 setting)')
  })

  it('renders every jump target as a titled section carrying its anchor', () => {
    renderWithProviders(<SettingsHub />)

    const headings = screen.getAllByRole('heading', { level: 2 })
    expect(headings.map(heading => heading.textContent)).toEqual(TITLES)

    SETTINGS_SECTIONS.forEach(section => {
      const region = screen.getByRole('region', { name: section.title })
      expect(region.id).toBe(section.anchor)
    })
  })

  it('links each unmoved control to the surface that owns it today', () => {
    renderWithProviders(<SettingsHub />)

    const account = screen.getByRole('region', { name: 'Account' })
    expect(
      within(account).getByRole('link', { name: 'Manage in profile settings' })
    ).toHaveAttribute('href', '/profile?tab=settings')
    expect(account).toHaveTextContent(
      'Email address · Password · Passkeys · Connected accounts · Export your data · Delete account'
    )

    // The shared action label is told apart by the settings its row names.
    expect(
      within(account).getByRole('link', {
        name: 'Manage in profile settings',
        description: /^Email address · Password/,
      })
    ).toBeInTheDocument()

    const alerts = screen.getByRole('region', { name: 'Alerts and email' })
    expect(
      within(alerts).getByRole('link', { name: 'Manage in profile settings' })
    ).toHaveAttribute('href', ALERTS_HREF)
    expect(
      within(alerts).getByRole('link', { name: 'Manage in custom alerts' })
    ).toHaveAttribute('href', '/settings/notification-filters')

    const appearance = screen.getByRole('region', { name: 'Appearance' })
    expect(
      within(appearance).getByRole('link', {
        name: 'Manage in appearance settings',
      })
    ).toHaveAttribute('href', '/settings/appearance')
  })

  it('mounts the live home layout list and the Your area row in Home page', () => {
    renderWithProviders(<SettingsHub />)

    const homePage = screen.getByRole('region', { name: 'Home page' })
    const list = within(homePage).getByRole('list', { name: 'Home sections' })
    expect(within(list).getAllByRole('listitem')).toHaveLength(
      HOME_SECTIONS.length
    )

    const area = homePage.querySelector('#alerts-area')
    expect(area).not.toBeNull()
    expect(area).toHaveTextContent('Your area')
    expect(
      within(area as HTMLElement).getByRole('link')
    ).toHaveAttribute('href', ALERTS_AREA_HREF)
  })

  it('scrolls to and focuses the section a cold-loaded fragment names', () => {
    setHash('#alerts')
    renderWithProviders(<SettingsHub />)

    const alerts = screen.getByRole('region', { name: 'Alerts and email' })
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    expect(scrollIntoView.mock.contexts[0]).toBe(alerts)
    expect(alerts).toHaveFocus()
    expect(alerts.className).toContain(
      'scroll-mt-[calc(var(--topbar-height)+1rem)]'
    )

    const rail = screen.getByRole('navigation', { name: 'Settings sections' })
    expect(
      within(rail).getByRole('link', { name: /^Alerts and email/ })
    ).toHaveAttribute('aria-current', 'true')
  })

  it('lands a Your area fragment on its row and marks the Home page section', () => {
    setHash('#alerts-area')
    renderWithProviders(<SettingsHub />)

    const area = document.getElementById('alerts-area')
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    expect(scrollIntoView.mock.contexts[0]).toBe(area)
    expect(area).toHaveFocus()

    const rail = screen.getByRole('navigation', { name: 'Settings sections' })
    expect(
      within(rail).getByRole('link', { name: /^Home page/ })
    ).toHaveAttribute('aria-current', 'true')
  })

  it('jumps within the hub when another hidden tree carries the same id', () => {
    const decoy = document.createElement('div')
    decoy.hidden = true
    decoy.innerHTML = '<div id="alerts" tabindex="-1"></div>'
    document.body.prepend(decoy)
    try {
      renderWithProviders(<SettingsHub />)
      const alerts = screen.getByRole('region', { name: 'Alerts and email' })

      for (const name of ['Settings sections', 'On this page']) {
        scrollIntoView.mockReset()
        const nav = screen.getByRole('navigation', { name })
        fireEvent.click(
          within(nav).getByRole('link', { name: /^Alerts and email/ })
        )
        expect(scrollIntoView.mock.contexts[0]).toBe(alerts)
        expect(alerts).toHaveFocus()
        expect(window.location.hash).toBe('#alerts')
        setHash('')
      }

      const rail = screen.getByRole('navigation', { name: 'Settings sections' })
      expect(
        within(rail).getByRole('link', { name: /^Alerts and email/ })
      ).toHaveAttribute('aria-current', 'true')
    } finally {
      decoy.remove()
    }
  })

  it('leaves a modified click to the browser', () => {
    renderWithProviders(<SettingsHub />)
    const rail = screen.getByRole('navigation', { name: 'Settings sections' })
    const link = within(rail).getByRole('link', { name: /^Feeds/ })

    const handled = !fireEvent.click(link, { metaKey: true })
    expect(handled).toBe(false)
    expect(scrollIntoView).not.toHaveBeenCalled()
  })

  it('marks the first section and scrolls nowhere without a fragment', () => {
    renderWithProviders(<SettingsHub />)

    expect(scrollIntoView).not.toHaveBeenCalled()
    const rail = screen.getByRole('navigation', { name: 'Settings sections' })
    const current = within(rail)
      .getAllByRole('link')
      .filter(link => link.getAttribute('aria-current') === 'true')
    expect(current).toHaveLength(1)
    expect(current[0]).toHaveTextContent('Account')
  })
})
