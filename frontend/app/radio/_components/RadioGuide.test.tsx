import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RadioGuide } from './RadioGuide'
import { localIso } from '@/features/radio/lib/localIso.testutil'
import type { RadioGuideRow } from '@/features/radio/types'

// Fixture instants built FROM local wall-clock (localIso) so the viewer-local
// assertions hold in any test-machine timezone (PSY-1298 convention). The
// station timezone is pinned to the VIEWER's zone so the station-local aside
// is deterministically absent in the main fixtures.
const viewerZone = Intl.DateTimeFormat().resolvedOptions().timeZone

function row(overrides: Partial<RadioGuideRow> = {}): RadioGuideRow {
  return {
    station: { slug: 'wfmu', name: 'WFMU 91.1' },
    show: { id: 1, slug: 'wake', name: 'Wake', host_name: null },
    starts_at: localIso(2026, 6, 8, 15),
    ends_at: localIso(2026, 6, 8, 18),
    station_timezone: viewerZone,
    ...overrides,
  }
}

describe('RadioGuide', () => {
  it('renders ON NOW and UP NEXT groups with show/station links and viewer-local times', () => {
    render(
      <RadioGuide
        onNow={[row()]}
        upNext={[
          row({
            show: { id: 2, slug: 'polyglot', name: 'Polyglot', host_name: 'DJ Mona' },
            starts_at: localIso(2026, 6, 8, 18),
            ends_at: localIso(2026, 6, 8, 21),
          }),
        ]}
      />
    )

    expect(screen.getByText('On now')).toBeInTheDocument()
    expect(screen.getByText('Up next')).toBeInTheDocument()

    const showLink = screen.getByRole('link', { name: 'Wake' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/wake')
    expect(screen.getAllByRole('link', { name: 'WFMU 91.1' })[0]).toHaveAttribute(
      'href',
      '/radio/wfmu'
    )

    // Viewer-local compact range (3–6 PM local by construction).
    expect(screen.getByText(/3–6 PM/)).toBeInTheDocument()
    // Host renders when present.
    expect(screen.getByText(/DJ Mona/)).toBeInTheDocument()
    // The honesty caption is part of the section contract.
    expect(screen.getByText(/Scheduled programming only/)).toBeInTheDocument()
  })

  it('renders a station-local aside when the station zone differs from the viewer zone', () => {
    // Pick a zone that is never the CI machine's local zone assumption-free:
    // if the viewer happens to BE in Pacific/Kiritimati (UTC+14, no DST),
    // the aside is legitimately absent — guard the assertion instead of
    // producing a machine-dependent failure.
    const stationZone = 'Pacific/Kiritimati'
    render(<RadioGuide onNow={[row({ station_timezone: stationZone })]} upNext={[]} />)
    if (viewerZone !== stationZone) {
      // The aside is the parenthesized station-zone range with a zone label.
      expect(screen.getByText(/\(.+GMT\+14.*\)/)).toBeInTheDocument()
    }
  })

  it("marks a next-day UP NEXT row with 'tomorrow'", () => {
    const t = new Date()
    render(
      <RadioGuide
        onNow={[]}
        upNext={[
          row({
            starts_at: localIso(
              t.getFullYear(), t.getMonth(), t.getDate() + 1, 9),
            ends_at: localIso(
              t.getFullYear(), t.getMonth(), t.getDate() + 1, 12),
          }),
        ]}
      />
    )
    expect(screen.getByText(/tomorrow/)).toBeInTheDocument()
  })

  it('renders nothing when both groups are empty', () => {
    const { container } = render(<RadioGuide onNow={[]} upNext={null} />)
    expect(container).toBeEmptyDOMElement()
  })
})
