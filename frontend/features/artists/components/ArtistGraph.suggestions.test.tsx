import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderWithProviders } from '@/test/utils'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// PSY-1273: cover the canvas PAINT wiring that jsdom can't render — the suggested-direction
// affordance (nodeCanvasObject) and the DOI→label-priority feed (onRenderFramePost). The DOI
// MATH is unit-tested in graphDoi.test.ts; this guards the glue: a refactor that dropped the
// glow's zoom gate, unbalanced the save/restore, or stopped feeding DOI into label priority
// would silently regress with nothing else failing. Same prop-capture approach as
// ForceGraphView.hoverFocus.test.tsx / ArtistGraphVisualization.test.tsx.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let forceGraphProps: any = null
vi.mock('next/dynamic', () => ({
  default: () =>
    function ForceGraph2DStub(props: Record<string, unknown>) {
      forceGraphProps = props
      return <div data-testid="force-graph" />
    },
}))

vi.mock('next/link', () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}))

// Spy renderGraphLabels to capture the label specs (with their DOI-derived priority); keep
// degreeMap real (the component still imports it for the no-DOI fallback path).
const renderGraphLabelsSpy = vi.fn()
vi.mock('@/components/graph/graphLabels', async importOriginal => {
  const actual = await importOriginal<typeof import('@/components/graph/graphLabels')>()
  return { ...actual, renderGraphLabels: (...args: unknown[]) => renderGraphLabelsSpy(...args) }
})

import { ArtistGraphVisualization } from './ArtistGraph'
import { computeGraphDoi } from './graphDoi'
import { mergeEgoGraphs } from './mergeEgoGraphs'

const center: ArtistGraphNode = { id: 1, name: 'Center', slug: 'center', upcoming_show_count: 0 }
const sat = (id: number, name: string): ArtistGraphNode => ({ id, name, slug: `s${id}`, upcoming_show_count: 0 })
const simLink = (t: number): ArtistGraphLink => ({ source_id: 1, target_id: t, type: 'similar', score: 0.5, votes_up: 0, votes_down: 0 })

const data: ArtistGraph = {
  center,
  nodes: [sat(2, 'Alpha'), sat(3, 'Bravo')],
  links: [simLink(2), simLink(3)],
  user_votes: {},
}

// A render-shaped node for the captured nodeCanvasObject (it reads id/x/y/isCenter/show count).
const renderNode = (id: number) => ({ id, name: `n${id}`, slug: `n${id}`, upcoming_show_count: 0, isCenter: false, val: 4, x: 0, y: 0 })

// Recording canvas ctx: tracks save/restore balance, shadowBlur assignments, globalAlpha, and
// which path methods ran (moveTo/lineTo ⇒ the "+" badge — nothing else in nodeCanvasObject uses them).
function makeRecordingCtx() {
  const calls: string[] = []
  const shadowBlurs: number[] = []
  const alphas: number[] = []
  let _alpha = 1
  let _shadowBlur = 0
  return {
    save() { calls.push('save') },
    restore() { calls.push('restore') },
    beginPath() {}, arc() { calls.push('arc') }, fill() {}, stroke() {},
    moveTo() { calls.push('moveTo') }, lineTo() { calls.push('lineTo') }, setLineDash() {},
    fillStyle: '', strokeStyle: '', lineWidth: 0, shadowColor: '',
    get shadowBlur() { return _shadowBlur },
    set shadowBlur(v: number) { _shadowBlur = v; shadowBlurs.push(v) },
    get globalAlpha() { return _alpha },
    set globalAlpha(v: number) { _alpha = v; alphas.push(v) },
    calls, shadowBlurs, alphas,
  }
}

const render = (props: Partial<Parameters<typeof ArtistGraphVisualization>[0]> = {}) =>
  renderWithProviders(
    <ArtistGraphVisualization
      data={data}
      activeTypes={new Set(['similar'])}
      containerWidth={1024}
      {...props}
    />,
  )

describe('ArtistGraphVisualization — suggested-direction paint (PSY-1273)', () => {
  beforeEach(() => { forceGraphProps = null; renderGraphLabelsSpy.mockClear() })
  afterEach(() => { vi.clearAllMocks(); vi.clearAllTimers() })

  it('draws the "+" badge for a suggested node, with the glow only at readable zoom', () => {
    render({ suggestedIds: new Set([2]), doiByNodeId: new Map([[2, 0.9], [3, 0.1]]) })

    // Readable zoom: badge ("+") drawn AND the soft glow (shadowBlur) applied.
    const hi = makeRecordingCtx()
    forceGraphProps.nodeCanvasObject(renderNode(2), hi as unknown as CanvasRenderingContext2D, 1.0)
    expect(hi.calls).toContain('moveTo') // the "+" strokes
    expect(hi.shadowBlurs.some(v => v > 0)).toBe(true) // glow on
    expect(hi.calls.filter(c => c === 'save').length).toBe(hi.calls.filter(c => c === 'restore').length) // balanced
    expect(hi.alphas[hi.alphas.length - 1]).toBe(1) // globalAlpha reset

    // Far zoom-out: badge STILL drawn (the hint survives the dense multi-expand view), glow OFF
    // (a device-px shadowBlur would bloom over tiny dots).
    const lo = makeRecordingCtx()
    forceGraphProps.nodeCanvasObject(renderNode(2), lo as unknown as CanvasRenderingContext2D, 0.5)
    expect(lo.calls).toContain('moveTo') // badge still there
    expect(lo.shadowBlurs).toEqual([]) // glow gate skipped the shadowBlur assignment entirely
  })

  it('does NOT draw the badge for a non-suggested node', () => {
    render({ suggestedIds: new Set([2]), doiByNodeId: new Map([[2, 0.9], [3, 0.1]]) })
    const ctx = makeRecordingCtx()
    forceGraphProps.nodeCanvasObject(renderNode(3), ctx as unknown as CanvasRenderingContext2D, 1.0)
    expect(ctx.calls).not.toContain('moveTo') // no "+" badge on an un-suggested node
  })

  it('feeds DOI into label collision priority (most-interesting names win the cull)', () => {
    render({ suggestedIds: new Set<number>(), doiByNodeId: new Map([[2, 0.9], [3, 0.1]]) })
    const ctx = makeRecordingCtx()
    // onRenderFramePost === nodeLabelsFrame; globalScale > LABEL_MIN_SCALE so it runs.
    forceGraphProps.onRenderFramePost(ctx as unknown as CanvasRenderingContext2D, 1.0)
    expect(renderGraphLabelsSpy).toHaveBeenCalled()
    const specs = renderGraphLabelsSpy.mock.calls[0][2] as Array<{ text: string; priority?: number }>
    const alpha = specs.find(s => s.text === 'Alpha') // node 2 (DOI 0.9)
    const bravo = specs.find(s => s.text === 'Bravo') // node 3 (DOI 0.1)
    expect(alpha?.priority).toBeGreaterThan(bravo?.priority ?? 0)
  })
})

// PSY-1273 (adversarial round 2): the feature's correctness rests on DOI scoring exactly the
// nodes the canvas paints — otherwise a node could glow / win a label on edges that aren't
// drawn. That equality holds because both paths filter the SAME merged.links by the SAME
// activeTypes and cap with the SAME EDGE_CAP_BY_TYPE — but that's a cross-component agreement
// no single file enforces. This pins it end-to-end: the canvas's real graphData projection
// (painted set, captured off the prop) must equal computeGraphDoi's scored set. A future change
// to either filter/cap site that breaks the agreement fails here instead of shipping silently.
const radio = (s: number, t: number, score: number): ArtistGraphLink => ({ source_id: s, target_id: t, type: 'radio_cooccurrence', score, votes_up: 0, votes_down: 0 })

describe('ArtistGraphVisualization — DOI scored set equals the painted set (PSY-1273)', () => {
  beforeEach(() => { forceGraphProps = null })
  afterEach(() => { vi.clearAllMocks(); vi.clearAllTimers() })

  const paintedSatelliteIds = (): number[] =>
    (forceGraphProps.graphData.nodes as Array<{ id: number; isCenter: boolean }>)
      .filter(n => !n.isCenter).map(n => n.id).sort((a, b) => a - b)

  it('a dense radio graph: every painted satellite is scored, and vice versa', () => {
    // center + 6 satellites, each tied to center AND densely cross-connected, so the cap trims
    // weak cross-ties — but no node is orphaned, so all 6 stay painted AND scored.
    const links: ArtistGraphLink[] = []
    for (let i = 2; i <= 7; i++) links.push(radio(1, i, 0.95))
    let s = 0.9
    for (let i = 2; i <= 7; i++) for (let j = i + 1; j <= 7; j++) { s -= 0.01; links.push(radio(i, j, s)) }
    const merged = mergeEgoGraphs(
      { center, nodes: [2, 3, 4, 5, 6, 7].map(i => sat(i, `n${i}`)), links, user_votes: {} },
      new Map(),
    )
    const data = { center: merged.center, nodes: merged.nodes, links: merged.links, user_votes: {} }
    const activeTypes = new Set(['radio_cooccurrence'])
    render({ data, activeTypes, doiByNodeId: computeGraphDoi(merged, activeTypes).doiByNodeId })

    const scored = [...computeGraphDoi(merged, activeTypes).doiByNodeId.keys()].sort((a, b) => a - b)
    expect(paintedSatelliteIds()).toEqual(scored)
    expect(scored).toEqual([2, 3, 4, 5, 6, 7])
  })

  it('a toggled-off edge type drops the same node from BOTH the painted set and the scored set', () => {
    // node 2 reachable via similar, node 3 only via radio. With activeTypes = {similar}, node 3
    // has no drawn edge → not painted; it must also be unscored (else a suggestion/label for a
    // node the canvas never drew).
    const merged = mergeEgoGraphs(
      { center, nodes: [sat(2, 'n2'), sat(3, 'n3')], links: [simLink(2), radio(1, 3, 0.9)], user_votes: {} },
      new Map(),
    )
    const data = { center: merged.center, nodes: merged.nodes, links: merged.links, user_votes: {} }
    const activeTypes = new Set(['similar'])
    render({ data, activeTypes, doiByNodeId: computeGraphDoi(merged, activeTypes).doiByNodeId })

    const scored = [...computeGraphDoi(merged, activeTypes).doiByNodeId.keys()].sort((a, b) => a - b)
    expect(paintedSatelliteIds()).toEqual([2])
    expect(scored).toEqual([2])
  })
})
