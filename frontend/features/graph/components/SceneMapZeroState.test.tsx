import { afterAll, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { renderWithProviders } from '@/test/utils'

import { formatLastMapped } from '../graphWeek'
import type { SceneMap, SceneMapNode } from '../sceneMap'
import { SceneMapZeroState } from './SceneMapZeroState'

// The runner's own zone. Written below the imports but RUN BEFORE them — that
// is what `vi.hoisted` buys, and it is the point: `formatLastMapped` builds its
// `Intl.DateTimeFormat` at module scope, so the zone has to be set before the
// import above evaluates.
//
// Without this the footer's date test is vacuous on CI, whose boxes are UTC:
// there, a footer that had gone back to formatting the instant itself produces
// the identical string and the test stays green with the bug fully restored.
// Phoenix because it is UTC-7 and never observes DST, so the offset that makes
// the assertion bite is the same in August as in January.
//
// Nothing else in this file asserts a date, so the pin is inert for the rest.
//
// Restored in `afterAll` because `process.env` is per-PROCESS while the module
// registry is per-file: vitest's default `isolate: true` gives each file its
// own fork and contains this, but under `--isolate=false` the pin would outlive
// the file and silently retime whichever ambient-zone suite ran next.
const { originalTz } = vi.hoisted(() => {
  const originalTz = process.env.TZ
  process.env.TZ = 'America/Phoenix'
  return { originalTz }
})

afterAll(() => {
  if (originalTz === undefined) delete process.env.TZ
  else process.env.TZ = originalTz
})

// jsdom cannot render a canvas, so the map surface is stubbed down to the
// callbacks the host wires: this file covers the CARD around the map (band,
// footer, list, mobile branch, hub panel), not the paint.
const { canvasProps } = vi.hoisted(() => ({
  canvasProps: { current: null as Record<string, unknown> | null },
}))

vi.mock('./SceneMapCanvas', () => ({
  SceneMapCanvas: (props: {
    map: SceneMap
    onSelectArtist: (node: SceneMapNode) => void
    onSelectHub: (node: SceneMapNode) => void
    ariaLabel: string
  }) => {
    canvasProps.current = props
    return (
      <div aria-label={props.ariaLabel}>
        <button type="button" onClick={() => props.onSelectArtist(props.map.nodes[0])}>
          Click artist dot
        </button>
        <button type="button" onClick={() => props.onSelectHub(props.map.nodes[3])}>
          Click hub
        </button>
      </div>
    )
  },
}))

function node(overrides: Partial<SceneMapNode> & Pick<SceneMapNode, 'id' | 'name'>): SceneMapNode {
  return {
    kind: 'artist',
    slug: overrides.name.toLowerCase(),
    x: 0,
    y: 0,
    community: 7,
    degree: 1,
    rank: 0,
    hasUpcomingShow: false,
    hasPlayableAudio: false,
    homeCity: null,
    appear: 0,
    ...overrides,
  }
}

function sceneMapFixture(overrides: Partial<SceneMap> = {}): SceneMap {
  const nodes = [
    node({ id: 1, name: 'Alpha', rank: 0 }),
    node({ id: 2, name: 'Beta', rank: 1, hasUpcomingShow: true }),
    node({ id: 3, name: 'Gamma', rank: 2, community: 9 }),
    node({
      id: 900001,
      name: 'Doom Records',
      slug: 'doom-records',
      kind: 'label',
      community: -1,
      degree: 4,
      homeCity: 'Brooklyn',
    }),
  ]
  return {
    nodes,
    edges: [],
    regions: [
      { community: 7, label: 'Around Alpha', memberCount: 2, hull: [], captionAnchor: null },
      { community: 9, label: 'Around Gamma', memberCount: 1, hull: [], captionAnchor: null },
    ],
    artistCount: 3,
    labelCount: 1,
    isolateCount: 42,
    lastMapped: new Date('2026-08-02T04:00:00Z'),
    epoch: new Date('2020-01-01T00:00:00Z'),
    ...overrides,
  }
}

function renderZeroState(
  props: Partial<React.ComponentProps<typeof SceneMapZeroState>> = {},
) {
  const onSelectArtist = vi.fn()
  renderWithProviders(
    <SceneMapZeroState
      map={sceneMapFixture()}
      canvasWidth={1024}
      onSelectArtist={onSelectArtist}
      {...props}
    />,
  )
  return { onSelectArtist }
}

describe('SceneMapZeroState', () => {
  it('reports the isolate count and points at the one action that changes it', () => {
    renderZeroState()

    expect(screen.getByText(/\+42 not yet connected artists/)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Add a show or a label to put one on the map/ }),
    ).toHaveAttribute('href', '/contribute')
  })

  it('hides the isolate band when every artist is on the map', () => {
    renderZeroState({ map: sceneMapFixture({ isolateCount: 0 }) })

    expect(screen.queryByText(/not yet connected/)).not.toBeInTheDocument()
  })

  it('singularizes the isolate band for one artist', () => {
    renderZeroState({ map: sceneMapFixture({ isolateCount: 1 }) })

    expect(screen.getByText(/\+1 not yet connected artist\./)).toBeInTheDocument()
  })

  it('states when the map was built and what is on it', () => {
    renderZeroState()

    expect(screen.getByText(/Mapped nightly · Last mapped/)).toBeInTheDocument()
    expect(screen.getByText(/3 connected · 1 labels · 2 regions/)).toBeInTheDocument()
  })

  it('dates the snapshot through the formatter the week maths shares', () => {
    // A wiring check, not a second copy of the timezone rule: that rule and its
    // midnight boundary are pinned once, on `formatLastMapped` in
    // `graphWeek.test.ts`. What can regress HERE is the footer going back to
    // formatting the instant itself, which is how it came to name the day
    // before its own week (00:30 UTC on the 2nd is the 1st across the Americas).
    const lastMapped = new Date('2026-08-02T00:30:00Z')
    renderZeroState({ map: sceneMapFixture({ lastMapped }) })

    expect(screen.getByText(/Mapped nightly · Last mapped/)).toHaveTextContent(
      formatLastMapped(lastMapped)
    )
  })

  it('re-roots on an artist dot exactly as search does', async () => {
    const user = userEvent.setup()
    const { onSelectArtist } = renderZeroState()

    await user.click(screen.getByRole('button', { name: 'Click artist dot' }))

    expect(onSelectArtist).toHaveBeenCalledWith({ id: 1, slug: 'alpha', name: 'Alpha' })
  })

  it('opens the entity panel for a label hub, and closes it on a second click', async () => {
    const user = userEvent.setup()
    renderZeroState()

    await user.click(screen.getByRole('button', { name: 'Click hub' }))
    const panel = screen.getByRole('region', { name: 'About Doom Records' })
    expect(panel).toBeInTheDocument()
    expect(panel).toHaveTextContent('Doom Records')
    expect(panel).toHaveTextContent('4 artists on the map')
    expect(panel.querySelector('a')).toHaveAttribute('href', '/labels/doom-records')

    await user.click(screen.getByRole('button', { name: 'Click hub' }))
    expect(
      screen.queryByRole('region', { name: 'About Doom Records' }),
    ).not.toBeInTheDocument()
  })

  it('states the hub home city in its panel', async () => {
    // The panel and the canvas caption read the SAME field (PSY-1736), so the
    // dot and the card cannot disagree about where a label is from.
    const user = userEvent.setup()
    renderZeroState()

    await user.click(screen.getByRole('button', { name: 'Click hub' }))

    expect(screen.getByRole('region', { name: 'About Doom Records' })).toHaveTextContent(
      'Brooklyn',
    )
  })

  it('omits the panel city line for a hub with no city on file', async () => {
    // No placeholder: a label known only by name drops the line rather than
    // saying "location unknown".
    const user = userEvent.setup()
    const map = sceneMapFixture()
    map.nodes[3] = { ...map.nodes[3], homeCity: null }
    renderZeroState({ map })

    await user.click(screen.getByRole('button', { name: 'Click hub' }))

    expect(
      screen.getByRole('region', { name: 'About Doom Records' }),
    ).not.toHaveTextContent('Brooklyn')
  })

  it('does not open an entity panel for an artist dot', async () => {
    const user = userEvent.setup()
    renderZeroState()

    await user.click(screen.getByRole('button', { name: 'Click artist dot' }))

    expect(screen.queryByRole('region', { name: /^About / })).not.toBeInTheDocument()
  })

  it('lists the map by region, so the canvas is not the only way in', async () => {
    const user = userEvent.setup()
    const { onSelectArtist } = renderZeroState()

    await user.click(screen.getByText('Browse the map as a list'))
    await user.click(screen.getByText('Around Alpha'))
    await user.click(screen.getByRole('button', { name: /Beta/ }))

    expect(onSelectArtist).toHaveBeenCalledWith({ id: 2, slug: 'beta', name: 'Beta' })
  })

  it('opens a second region without closing it again', async () => {
    // Re-rendering the first region with open={false} fires its own toggle
    // event. A handler that reads that as "nothing is open" shuts the region
    // the visitor just clicked, so every region after the first needs two
    // clicks — on the surface that IS the map on a phone.
    const user = userEvent.setup()
    renderZeroState()

    await user.click(screen.getByText('Browse the map as a list'))
    await user.click(screen.getByText('Around Alpha'))
    expect(screen.getByRole('button', { name: /Beta/ })).toBeInTheDocument()

    await user.click(screen.getByText('Around Gamma'))

    expect(screen.getByRole('button', { name: /Gamma/ })).toBeInTheDocument()
    // ...and the first region gave up its rows, so only one is mounted.
    expect(screen.queryByRole('button', { name: /Beta/ })).not.toBeInTheDocument()
  })

  describe('the this-week share affordance', () => {
    // The fixture's nodes all read `appear: 0`, which is how an undated snapshot
    // arrives — so the default map has no week and the base cases above are the
    // "no affordance" cases. These fixtures date the arrivals instead.
    const EPOCH = new Date('2020-01-01T00:00:00Z')
    const appearAt = (iso: string) =>
      Math.floor((new Date(iso).getTime() - EPOCH.getTime()) / 1000)

    /** A map whose window (JUL 27 - AUG 2) contains `count` arrivals. */
    function datedMap(arrivals: string[]) {
      const base = sceneMapFixture()
      return sceneMapFixture({
        nodes: base.nodes.map((entry, index) => ({
          ...entry,
          appear: appearAt(arrivals[index] ?? '2021-05-05T00:00:00Z'),
        })),
      })
    }

    it('offers the share link when something arrived this week', () => {
      renderZeroState({ map: datedMap(['2026-07-30T00:00:00Z', '2026-08-01T00:00:00Z']) })

      const link = screen.getByRole('link', { name: /Share this week in the graph/ })
      expect(link).toHaveAttribute('href', '/graph/this-week')
      // The accessible name carries the numbers, because the visible chip says
      // only "This week" and the counts are the point.
      expect(link).toHaveAccessibleName(
        /2 new artists and 0 new connections joined the map, JUL 27 - AUG 2 2026\./,
      )
    })

    it('stays out of the way on a week with nothing new', () => {
      // Dateable, so `/graph/this-week` would render — but a `+0 ARTISTS` card is
      // not something to invite anyone to share.
      renderZeroState({ map: datedMap(['2021-01-01T00:00:00Z']) })

      expect(
        screen.queryByRole('link', { name: /Share this week in the graph/ }),
      ).not.toBeInTheDocument()
      // The rest of the freshness row is untouched.
      expect(screen.getByText(/Mapped nightly · Last mapped/)).toBeInTheDocument()
    })

    it('stays out of the way when the snapshot carries no arrival dates', () => {
      renderZeroState()

      expect(
        screen.queryByRole('link', { name: /Share this week in the graph/ }),
      ).not.toBeInTheDocument()
    })

    it('goes away with the rest of the freshness strip during a replay', () => {
      const replay = {
        timeline: { bins: [] },
        isActive: true,
        start: vi.fn(),
        exit: vi.fn(),
        readFrame: vi.fn(),
        subscribe: vi.fn(() => () => {}),
        seek: vi.fn(),
        setScrubbing: vi.fn(),
        togglePause: vi.fn(),
        phase: 'playing',
      } as unknown as React.ComponentProps<typeof SceneMapZeroState>['replay']

      renderZeroState({
        map: datedMap(['2026-07-30T00:00:00Z']),
        replay,
      })

      expect(
        screen.queryByRole('link', { name: /Share this week in the graph/ }),
      ).not.toBeInTheDocument()
    })

    it('is offered below the canvas breakpoint too', () => {
      // Unlike the replay chip, sharing a card has nothing to do with there
      // being a canvas — a phone is where a link gets posted.
      renderZeroState({
        canvasWidth: null,
        map: datedMap(['2026-07-30T00:00:00Z']),
      })

      expect(
        screen.getByRole('link', { name: /Share this week in the graph/ }),
      ).toHaveAttribute('href', '/graph/this-week')
    })
  })

  describe('with no canvas (below the breakpoint)', () => {
    it('replaces the map with a pitch line carrying the live counts', () => {
      renderZeroState({ canvasWidth: null })

      expect(screen.queryByRole('button', { name: 'Click artist dot' })).not.toBeInTheDocument()
      expect(
        screen.getByText(/3 artists are already connected across 2 regions of the scene\./),
      ).toBeInTheDocument()
    })

    it('links the pitch line at the list, and still renders the list', async () => {
      const user = userEvent.setup()
      const { onSelectArtist } = renderZeroState({ canvasWidth: null })

      expect(
        screen.getByRole('link', { name: 'Browse the map as a list' }),
      ).toHaveAttribute('href', '#scene-map-list')

      await user.click(screen.getByText('Browse the map as a list', { selector: 'summary' }))
      await user.click(screen.getByText('Around Gamma'))
      await user.click(screen.getByRole('button', { name: /Gamma/ }))

      expect(onSelectArtist).toHaveBeenCalledWith({ id: 3, slug: 'gamma', name: 'Gamma' })
    })
  })
})
