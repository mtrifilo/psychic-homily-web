import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { HomeSceneGraph } from './HomeSceneGraph'
import type { SceneListItem } from '@/features/scenes'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

const push = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}))

// The dynamic(ssr:false) chunk never resolves in jsdom's sync render pass —
// stub the module so the section's own logic is what's under test. The
// static-viewport behavior itself is covered in
// components/graph/ForceGraphView.staticViewport.test.tsx.
vi.mock('@/components/graph/ForceGraphView', () => ({
  ForceGraphView: (props: { ariaLabel: string; staticViewport?: boolean }) => (
    <div
      data-testid="force-graph-view"
      data-static-viewport={String(props.staticViewport ?? false)}
      aria-label={props.ariaLabel}
    />
  ),
}))

const useScenes = vi.fn()
const useSceneGraph = vi.fn()
vi.mock('@/features/scenes', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/scenes')>()
  return {
    ...actual,
    useScenes: () => useScenes(),
    useSceneGraph: (opts: { slug: string; enabled?: boolean }) => useSceneGraph(opts),
  }
})

function scene(overrides: Partial<SceneListItem> & { slug: string }): SceneListItem {
  return {
    city: 'Phoenix',
    state: 'AZ',
    venue_count: 3,
    upcoming_show_count: 4,
    total_show_count: 10,
    shows_this_week: 1,
    ...overrides,
  }
}

const SCENES = [
  scene({ slug: 'chicago-il', city: 'Chicago', state: 'IL', upcoming_show_count: 17 }),
  scene({ slug: 'phoenix-az', upcoming_show_count: 4 }),
]

const GRAPH = {
  scene: {
    slug: 'chicago-il',
    city: 'Chicago',
    state: 'IL',
    artist_count: 2,
    edge_count: 1,
    metro_roster_total: 2,
    roster_truncated: false,
  },
  clusters: [],
  nodes: [
    { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 1, cluster_id: 'other', is_isolate: false },
    { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0, cluster_id: 'other', is_isolate: false },
  ],
  links: [{ source_id: 1, target_id: 2, type: 'shared_bill' }],
}

// test/setup.ts installs a never-intersecting IntersectionObserver mock —
// replace it (plain writable assignment; defineProperty/stubGlobal can't
// redefine it) with one that intersects immediately so the lazy section
// mounts in tests. ResizeObserver's setup no-op is fine as-is; the initial
// getBoundingClientRect measure drives useContainerWidth.
class ImmediateIntersectionObserver {
  constructor(private cb: IntersectionObserverCallback) {}
  observe() {
    this.cb(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    )
  }
  disconnect() {}
  unobserve() {}
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
}

function setContainerWidth(px: number) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    width: px, height: 0, top: 0, left: 0, bottom: 0, right: 0, x: 0, y: 0,
    toJSON: () => ({}),
  } as DOMRect)
}

beforeEach(() => {
  vi.restoreAllMocks()
  push.mockReset()
  window.IntersectionObserver =
    ImmediateIntersectionObserver as unknown as typeof IntersectionObserver
  useScenes.mockReset().mockReturnValue({ data: { scenes: SCENES, count: SCENES.length }, isLoading: false, isError: false })
  useSceneGraph.mockReset().mockReturnValue({ data: GRAPH, isLoading: false, isError: false })
  setContainerWidth(1024)
})

describe('HomeSceneGraph', () => {
  it('renders the liveliest scene in the heading with the CTA link to its scene page', async () => {
    render(<HomeSceneGraph />)
    expect(
      await screen.findByRole('heading', { name: 'The Chicago scene, mapped' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /explore the full graph/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('requests the graph in static-viewport (click-select only) mode with the shared count phrase in the aria-label', async () => {
    render(<HomeSceneGraph />)
    const graph = await screen.findByTestId('force-graph-view')
    expect(graph).toHaveAttribute('data-static-viewport', 'true')
    expect(graph).toHaveAttribute(
      'aria-label',
      expect.stringContaining('Knowledge graph of the Chicago scene: 2 artists'),
    )
  })

  it('"Surprise me" rotates to another scene', async () => {
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Chicago scene, mapped' })
    fireEvent.click(screen.getByRole('button', { name: /surprise me/i }))
    // Only one other scene exists, so the rotation is deterministic.
    expect(
      screen.getByRole('heading', { name: 'The Phoenix scene, mapped' }),
    ).toBeInTheDocument()
    expect(useSceneGraph).toHaveBeenLastCalledWith(
      expect.objectContaining({ slug: 'phoenix-az' }),
    )
  })

  it('self-hides entirely when the scenes list errors', async () => {
    useScenes.mockReturnValue({ data: undefined, isLoading: false, isError: true })
    const { container } = render(<HomeSceneGraph />)
    expect(screen.queryByRole('heading', { name: /scene, mapped/i })).toBeNull()
    // The wrapper stays (observer target) but carries no section content.
    expect(container.querySelector('section')).toBeNull()
  })

  it('self-hides when the scenes list is empty', () => {
    useScenes.mockReturnValue({ data: { scenes: [], count: 0 }, isLoading: false, isError: false })
    const { container } = render(<HomeSceneGraph />)
    expect(container.querySelector('section')).toBeNull()
  })

  it('renders the small-screen teaser (no canvas) below the graph breakpoint', async () => {
    setContainerWidth(500)
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Chicago scene, mapped' })
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
    expect(
      screen.getByRole('link', { name: /see the chicago scene/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('renders the empty-graph fallback instead of a canvas when the scene has no connected artists', async () => {
    useSceneGraph.mockReturnValue({
      data: { ...GRAPH, nodes: [], links: [] },
      isLoading: false,
      isError: false,
    })
    render(<HomeSceneGraph />)
    expect(
      await screen.findByText(/not enough connected artists in chicago/i),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
  })
})
