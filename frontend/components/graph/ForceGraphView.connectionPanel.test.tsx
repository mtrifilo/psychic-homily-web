import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

// PSY-1334: the click-to-inspect ConnectionPanel contract on ForceGraphView —
// edge click opens the pair card (opt-in via showConnectionPanel), background/
// node clicks close it, a second edge click re-targets, and the panel lists
// ALL typed connections for the pair (including types hidden from the sim).

const h = vi.hoisted(() => ({
  graph: {
    pauseAnimation: vi.fn(),
    resumeAnimation: vi.fn(),
    d3Force: vi.fn(),
    d3ReheatSimulation: vi.fn(),
    zoomToFit: vi.fn(),
    zoom: vi.fn(() => 1),
    centerAt: vi.fn(() => ({ x: 0, y: 0 })),
    getGraphBbox: vi.fn(() => ({ x: [-100, 100], y: [-100, 100] })),
  },
  lastProps: { value: null as Record<string, unknown> | null },
}))

vi.mock('next/dynamic', async () => {
  const React = await import('react')
  return {
    default: () =>
      React.forwardRef(function ForceGraph2DStub(
        props: Record<string, unknown>,
        ref: React.Ref<unknown>,
      ) {
        React.useImperativeHandle(ref, () => h.graph)
        React.useEffect(() => {
          h.lastProps.value = props
        })
        return React.createElement('canvas', { 'data-testid': 'stub-canvas' })
      }),
  }
})

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => false,
}))

// PSY-1335: opening the panel fetches provenance. Reject by default (rows
// stay text-only) so the rest of the suite exercises the phase-1 path;
// individual tests resolve it to assert the entity upgrade.
const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))
vi.mock('@/lib/api-base', () => ({
  API_BASE_URL: 'http://localhost:8080',
}))

import { ForceGraphView, type GraphNode } from './ForceGraphView'
import { BACKGROUND_ALPHA } from './graphFocus'

const nodes: GraphNode[] = [
  { id: 1, name: 'Dehd', slug: 'dehd', upcoming_show_count: 0 },
  { id: 2, name: 'Lifeguard', slug: 'lifeguard', upcoming_show_count: 0 },
  { id: 3, name: 'Horsegirl', slug: 'horsegirl', upcoming_show_count: 0 },
]

const links = [
  { source_id: 1, target_id: 2, type: 'shared_bills', score: 0.3, detail: { shared_count: 3 } },
  { source_id: 1, target_id: 2, type: 'shared_label', detail: { shared_count: 1, label_names: 'Fire Talk' } },
  { source_id: 2, target_id: 3, type: 'shared_bills', score: 0.1, detail: { shared_count: 1 } },
]

const baseProps = {
  nodes,
  links,
  containerWidth: 800,
  ariaLabel: 'test graph',
  onNodeClick: () => {},
  showConnectionPanel: true,
}

const renderGraph = (props: Partial<React.ComponentProps<typeof ForceGraphView>> = {}) =>
  renderWithProviders(<ForceGraphView {...baseProps} {...props} />)

const clickLink = (source: number, target: number, type = 'shared_bills') => {
  act(() => {
    ;(h.lastProps.value!.onLinkClick as (l: unknown) => void)({ source, target, type })
  })
}

beforeEach(() => {
  h.lastProps.value = null
  vi.clearAllMocks()
  mockApiRequest.mockRejectedValue(
    Object.assign(new Error('Not Found'), { status: 404 }),
  )
})

describe('ForceGraphView connection panel', () => {
  it('opens on edge click with EVERY typed connection for the pair', () => {
    renderGraph()
    clickLink(1, 2)
    expect(
      screen.getByRole('region', { name: 'Why Dehd and Lifeguard are connected' }),
    ).toBeInTheDocument()
    // Both types between the pair render, not just the clicked one.
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(screen.getByText('Shared Label')).toBeInTheDocument()
  })

  it('resolves endpoints when d3 has replaced ids with node objects', () => {
    renderGraph()
    act(() => {
      ;(h.lastProps.value!.onLinkClick as (l: unknown) => void)({
        source: { id: 1 },
        target: { id: 2 },
        type: 'shared_bills',
      })
    })
    expect(screen.getByRole('region', { name: /Dehd and Lifeguard/ })).toBeInTheDocument()
  })

  it('re-targets on a second edge click (one panel at a time)', () => {
    renderGraph()
    clickLink(1, 2)
    clickLink(2, 3)
    expect(screen.queryByRole('region', { name: /Dehd and Lifeguard/ })).not.toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Why Lifeguard and Horsegirl are connected' }),
    ).toBeInTheDocument()
  })

  it('forwards background clicks to the consumer onBackgroundClick (PSY-1345) after closing the panel', () => {
    const onBackgroundClick = vi.fn()
    renderGraph({ onBackgroundClick })
    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /connected/ })).toBeInTheDocument()
    act(() => {
      ;(h.lastProps.value!.onBackgroundClick as () => void)()
    })
    // Both the internal panel dismissal AND the consumer callback fire —
    // deleting either half of the composed lambda must fail this test.
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
    expect(onBackgroundClick).toHaveBeenCalledTimes(1)
  })

  it('closes on background click and on node click', () => {
    renderGraph()
    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /connected/ })).toBeInTheDocument()
    act(() => {
      ;(h.lastProps.value!.onBackgroundClick as () => void)()
    })
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()

    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /connected/ })).toBeInTheDocument()
    act(() => {
      ;(h.lastProps.value!.onNodeClick as (n: unknown) => void)(nodes[2])
    })
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('closes via Escape', () => {
    renderGraph()
    clickLink(1, 2)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('ignores edge clicks when the surface has not opted in', () => {
    renderGraph({ showConnectionPanel: false })
    clickLink(1, 2)
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('ignores clicks on untyped links (no provenance to show)', () => {
    renderGraph({ links: [{ source_id: 1, target_id: 2, type: '' }] })
    clickLink(1, 2, '')
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })

  it('upgrades rows to entity links when provenance resolves (PSY-1335)', async () => {
    mockApiRequest.mockResolvedValue({
      connections: [
        {
          type: 'shared_bills',
          score: 0.3,
          entities: [
            { kind: 'show', id: 9, slug: 'dehd-empty-bottle', name: 'Empty Bottle', date: '2026-05-14' },
          ],
          entity_total: 3,
        },
      ],
    })
    renderGraph()
    clickLink(1, 2)

    // Phase-1 text rows render immediately, entities arrive async.
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(
      await screen.findByRole('link', { name: '2026-05-14 · Empty Bottle' }),
    ).toHaveAttribute('href', '/shows/dehd-empty-bottle')
    expect(screen.getByText('and 2 more')).toBeInTheDocument()
    // The pair is fetched once, canonical orientation.
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/artists/1/relationships/2/provenance',
      { method: 'GET' },
    )
    // shared_label had no provenance — its text row is untouched.
    expect(screen.getByText('Shared Label')).toBeInTheDocument()
  })

  it('keeps phase-1 text rows when the provenance fetch errors', async () => {
    renderGraph() // beforeEach default: fetch rejects with 404
    clickLink(1, 2)
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(screen.getByText('Shared Label')).toBeInTheDocument()
    // Give the rejected query a beat — the panel must not blank or lose rows.
    await act(async () => {
      await Promise.resolve()
    })
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(screen.queryAllByRole('link', { name: /·/ })).toHaveLength(0)
  })

  it('widens the link hit target beyond the lib DEFAULT of 4', () => {
    renderGraph()
    // force-graph's hit test is linkWidth + linkHoverPrecision with a
    // default precision of 4 — a value of 4 here would be a no-op
    // (adversarial finding), so the widening must exceed it.
    expect(h.lastProps.value!.linkHoverPrecision).toBeGreaterThan(4)
  })

  it('Escape claims the key from outer layers (fullscreen overlay contract)', () => {
    renderGraph()
    clickLink(1, 2)
    // The panel listens in the CAPTURE phase and preventDefaults, so the
    // fullscreen overlay's bubble-phase Escape handler (which checks
    // defaultPrevented) leaves fullscreen intact — innermost layer first.
    const outerEscapes: boolean[] = []
    const outerListener = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented) outerEscapes.push(true)
    }
    document.addEventListener('keydown', outerListener)
    fireEvent.keyDown(document.body, { key: 'Escape' })
    document.removeEventListener('keydown', outerListener)
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
    expect(outerEscapes).toHaveLength(0)
  })

  it('solo filters the simulation to one type and restores the hidden set intact', () => {
    renderGraph({ showEdgeLegend: true })
    const simLinks = () =>
      (h.lastProps.value!.graphData as { links: Array<{ type: string }> }).links.map(l => l.type)

    // Hide shared_label via its row toggle.
    fireEvent.click(screen.getByRole('button', { name: /^Shared Label/ }))
    expect(simLinks()).toEqual(['shared_bills', 'shared_bills'])

    // Solo shared_label: solo WINS over the hidden set — only it renders.
    fireEvent.click(screen.getByRole('button', { name: 'Show only Shared Label connections' }))
    expect(simLinks()).toEqual(['shared_label'])
    expect(screen.getByText('Showing only Shared Label connections')).toBeInTheDocument()

    // While soloed, hide-toggles are disabled — no blind hidden-set edits.
    expect(screen.getByRole('button', { name: /^Shared Bills/ })).toBeDisabled()

    // Clear the solo: the PRIOR hidden set (shared_label hidden) is intact.
    fireEvent.click(screen.getByRole('button', { name: 'Show all connection types' }))
    expect(simLinks()).toEqual(['shared_bills', 'shared_bills'])
  })

  it('self-heals a solo whose type leaves the displayable set', () => {
    const { rerender } = renderGraph({ showEdgeLegend: true })
    fireEvent.click(screen.getByRole('button', { name: 'Show only Shared Label connections' }))
    expect(screen.getByText('Showing only Shared Label connections')).toBeInTheDocument()

    // The soloed type's edges leave the payload (e.g. its carrying cluster
    // hidden / data refresh) — the solo must clear rather than strand a
    // filter with no legend row to undo it (code-review finding).
    rerender(
      <ForceGraphView
        {...baseProps}
        showEdgeLegend
        links={links.filter(l => l.type !== 'shared_label')}
      />,
    )
    expect(screen.queryByText(/Showing only/)).not.toBeInTheDocument()
    const simLinks = (h.lastProps.value!.graphData as { links: Array<{ type: string }> }).links
    expect(simLinks.length).toBeGreaterThan(0) // graph not stuck empty
  })

  it('pins the focus-dim to the inspected pair’s endpoints while the panel is open (PSY-1478)', () => {
    renderGraph()
    const renderNode = (id: number) => ({
      id, name: `n${id}`, slug: `n${id}`, upcoming_show_count: 0,
      cluster_id: 'other', is_isolate: false, x: 0, y: 0,
    })
    const paintAlphas = (id: number) => {
      const alphas: number[] = []
      let alpha = 1
      const ctx = {
        get globalAlpha() { return alpha },
        set globalAlpha(v: number) { alpha = v; alphas.push(v) },
        beginPath() {}, arc() {}, fill() {}, stroke() {},
        fillStyle: '', strokeStyle: '', lineWidth: 0,
      }
      ;(h.lastProps.value!.nodeCanvasObject as (n: unknown, c: unknown) => void)(
        renderNode(id), ctx,
      )
      return alphas
    }

    clickLink(1, 2)
    // Endpoints 1 and 2 stay foreground; node 3 dims to the background alpha —
    // the panel's "why connected" card keeps its counterpart on canvas.
    expect(paintAlphas(1).every(a => a === 1)).toBe(true)
    expect(paintAlphas(2).every(a => a === 1)).toBe(true)
    expect(paintAlphas(3)[0]).toBe(BACKGROUND_ALPHA)

    // Closing the panel releases the pin.
    act(() => {
      ;(h.lastProps.value!.onBackgroundClick as () => void)()
    })
    expect(paintAlphas(3).every(a => a === 1)).toBe(true)
  })

  it('auto-closes when the pair leaves the payload (data refresh)', () => {
    const { rerender } = renderGraph()
    clickLink(1, 2)
    expect(screen.getByRole('region', { name: /Dehd and Lifeguard/ })).toBeInTheDocument()
    rerender(
      <ForceGraphView
        {...baseProps}
        nodes={nodes.filter(n => n.id !== 2)}
        links={links.filter(l => l.source_id !== 2 && l.target_id !== 2)}
      />,
    )
    expect(screen.queryByRole('region', { name: /connected/ })).not.toBeInTheDocument()
  })
})
