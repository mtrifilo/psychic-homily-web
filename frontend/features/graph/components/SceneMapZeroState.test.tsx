import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { renderWithProviders } from '@/test/utils'

import type { SceneMap, SceneMapNode } from '../sceneMap'
import { SceneMapZeroState } from './SceneMapZeroState'

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
