import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

/**
 * These tests pin the routing contract for `/charts/featured` (PSY-1501):
 * the STATIC segment must win over the sibling `[module]` DYNAMIC segment.
 *
 * Next resolves static segments ahead of dynamic ones, so `app/charts/featured`
 * is served by THIS route. The guard below proves the dynamic segment genuinely
 * rejects "featured" (404) — so if the static route ever regressed, the archive
 * would break loudly rather than silently fall through to a chart drill-down.
 */

const notFound = vi.fn()
vi.mock('next/navigation', () => ({
  notFound: () => notFound(),
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => null,
}))

// Keep the routing predicates (isChartModuleSlug / calendarWindowFromRoute)
// REAL so the dynamic segment's decision is exercised, not stubbed. Only the
// heavy page bodies are mocked.
vi.mock('@/features/charts', () => ({
  FeaturedArchivePage: () => <div data-testid="featured-archive" />,
  ChartsPage: () => <div data-testid="charts-page" />,
}))

vi.mock('@/features/charts/components/ChartDrilldownPage', () => ({
  ChartDrilldownPage: () => <div data-testid="chart-drilldown" />,
}))

import FeaturedRoute, { metadata } from './page'
import ModuleRoute from '../[module]/page'

describe('/charts/featured static route', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the previously-featured archive page', () => {
    render(<FeaturedRoute />)
    expect(screen.getByTestId('featured-archive')).toBeInTheDocument()
  })

  it('advertises the /charts/featured canonical URL', () => {
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/charts/featured'
    )
  })

  it('is NOT swallowed by the [module] dynamic segment (it 404s "featured")', async () => {
    await ModuleRoute({ params: Promise.resolve({ module: 'featured' }) })
    expect(notFound).toHaveBeenCalled()
  })

  it('control: the [module] segment still serves a real module slug', async () => {
    await ModuleRoute({
      params: Promise.resolve({ module: 'most-anticipated' }),
    })
    expect(notFound).not.toHaveBeenCalled()
  })
})
