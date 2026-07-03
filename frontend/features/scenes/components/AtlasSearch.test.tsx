import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListItem } from '../types'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

import { AtlasSearch } from './AtlasSearch'

const scenes: SceneListItem[] = [
  {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    venue_count: 11,
    upcoming_show_count: 42,
    total_show_count: 180,
    shows_this_week: 0,
    latitude: 33.448,
    longitude: -112.074,
  },
  {
    city: 'Chicago',
    state: 'IL',
    slug: 'chicago-il',
    venue_count: 9,
    upcoming_show_count: 283,
    total_show_count: 337,
    shows_this_week: 0,
    latitude: 41.88,
    longitude: -87.63,
  },
  {
    // Unplaceable — no coords; selecting it must NAVIGATE, never "fly".
    city: 'Faketown',
    state: 'ZZ',
    slug: 'faketown-zz',
    venue_count: 2,
    upcoming_show_count: 3,
    total_show_count: 3,
    shows_this_week: 0,
  },
]

describe('AtlasSearch (PSY-1310)', () => {
  const onPick = vi.fn()

  beforeEach(() => {
    onPick.mockReset()
    mockPush.mockReset()
  })

  function openSearch() {
    renderWithProviders(<AtlasSearch scenes={scenes} onPick={onPick} />)
    fireEvent.click(screen.getByRole('combobox', { name: /search scenes/i }))
  }

  it('lists scenes most-active-first when opened', () => {
    openSearch()
    const options = screen.getAllByRole('option')
    expect(options[0]).toHaveTextContent('Chicago, IL')
    expect(options[1]).toHaveTextContent('Phoenix, AZ')
    expect(options[2]).toHaveTextContent('Faketown, ZZ')
  })

  it('filters as the user types', () => {
    openSearch()
    fireEvent.change(screen.getByPlaceholderText('City or state…'), {
      target: { value: 'phoe' },
    })
    expect(screen.getByRole('option', { name: /Phoenix/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Chicago/ })).not.toBeInTheDocument()
  })

  it('picking a placeable scene calls onPick (fly + preview), not navigation', () => {
    openSearch()
    fireEvent.click(screen.getByRole('option', { name: /Phoenix/ }))
    expect(onPick).toHaveBeenCalledTimes(1)
    expect(onPick.mock.calls[0][0]).toMatchObject({ slug: 'phoenix-az' })
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('picking an unplaceable scene navigates to its scene page instead', () => {
    openSearch()
    fireEvent.click(screen.getByRole('option', { name: /Faketown/ }))
    expect(mockPush).toHaveBeenCalledWith('/scenes/faketown-zz')
    expect(onPick).not.toHaveBeenCalled()
  })

  it('parks focus on the trigger when a scene is picked (PSY-1313 focus-return seam)', () => {
    // The preview panel captures document.activeElement at mount as its
    // focus-return target — a pick must leave the trigger focused, not the
    // cmdk input (which unmounts with the popover's exit animation).
    openSearch()
    fireEvent.click(screen.getByRole('option', { name: /Phoenix/ }))
    expect(
      screen.getByRole('combobox', { name: /search scenes/i }),
    ).toHaveFocus()
  })

  it('"/" opens the search — but not while typing in another field', () => {
    renderWithProviders(
      <>
        <input aria-label="decoy" />
        <AtlasSearch scenes={scenes} onPick={onPick} />
      </>,
    )
    // Typing "/" inside another input must NOT hijack it.
    fireEvent.keyDown(screen.getByLabelText('decoy'), { key: '/' })
    expect(screen.queryByRole('option')).not.toBeInTheDocument()

    // "/" on the document opens the combobox list.
    fireEvent.keyDown(document, { key: '/' })
    expect(screen.getAllByRole('option').length).toBeGreaterThan(0)
  })
})
