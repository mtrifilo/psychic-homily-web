import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { HomeLayoutRuntime } from './HomeLayoutRuntime'
import {
  moveHomeSection,
  resolveHomeLayout,
  setHomeSectionVisibility,
  type ResolvedHomeSection,
} from '../sections'

let layout: ResolvedHomeSection[] = resolveHomeLayout(null)

vi.mock('../hooks/useHomeLayout', () => ({
  useHomeLayout: () => layout,
  usePersistHomeLayout: () => ({
    persist: vi.fn(),
    reset: vi.fn(),
    isResetting: false,
    hasError: false,
  }),
}))

vi.mock('./HomeCityShowsLink', async importOriginal => ({
  ...(await importOriginal<object>()),
  ResolvedHomeCityShowsLink: () => (
    <span data-testid="relocated-city-link">All upcoming shows →</span>
  ),
}))

const SECTIONS = {
  saved_shows: <div data-testid="section">saved_shows</div>,
  nearby_shows: <div data-testid="section">nearby_shows</div>,
  community_stats: <div data-testid="section">community_stats</div>,
  city_graph: <div data-testid="section">city_graph</div>,
  radio_shows: <div data-testid="section">radio_shows</div>,
}

function renderedOrder() {
  return screen.queryAllByTestId('section').map(node => node.textContent)
}

beforeEach(() => {
  layout = resolveHomeLayout(null)
})

describe('HomeLayoutRuntime', () => {
  it('renders the toolbar and the shipped order', () => {
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(
      screen.getByText('Home · 5 sections · Your layout')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /customize home/i })
    ).toBeInTheDocument()
    expect(renderedOrder()).toEqual([
      'saved_shows',
      'nearby_shows',
      'community_stats',
      'city_graph',
      'radio_shows',
    ])
  })

  it('renders a custom order on the FIRST paint, with no reflow', () => {
    layout = moveHomeSection(resolveHomeLayout(null), 'radio_shows', 'up')

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([
      'saved_shows',
      'nearby_shows',
      'community_stats',
      'radio_shows',
      'city_graph',
    ])
  })

  it('does not mount a hidden section', () => {
    layout = setHomeSectionVisibility(
      resolveHomeLayout(null),
      'community_stats',
      false
    )

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([
      'saved_shows',
      'nearby_shows',
      'city_graph',
      'radio_shows',
    ])
  })

  it('keeps the toolbar and shows one line when every section is hidden', () => {
    layout = resolveHomeLayout(null).map(section => ({
      ...section,
      visible: false,
    }))

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(renderedOrder()).toEqual([])
    expect(
      screen.getByText(/You have hidden every section\./)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Customize home →' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /^Customize home$/i })
    ).toBeInTheDocument()
  })

  it('carries the city link on the toolbar only when both carriers are hidden', () => {
    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)
    expect(screen.queryByTestId('relocated-city-link')).not.toBeInTheDocument()
  })

  it('moves the city link to the toolbar when saved and nearby are hidden', () => {
    layout = setHomeSectionVisibility(
      setHomeSectionVisibility(resolveHomeLayout(null), 'nearby_shows', false),
      'saved_shows',
      false
    )

    render(<HomeLayoutRuntime initialLayout={null} sections={SECTIONS} />)

    expect(screen.getByTestId('relocated-city-link')).toBeInTheDocument()
  })
})
