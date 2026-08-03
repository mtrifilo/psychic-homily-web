'use client'

/**
 * PSY-1724 spike — canvas-wall bench. THROWAWAY. Never merged.
 *
 * Renders a synthetic precomputed-position graph through the SAME
 * react-force-graph-2d path the product uses (dynamic import, ssr:false,
 * nodeCanvasObject, onRenderFramePost decoration) with every node pinned
 * (fx/fy) and zero simulation (warmupTicks=0, cooldownTicks=0), so the only
 * thing under measurement is canvas 2D RENDERING.
 *
 * Query params:
 *   n=3000            node count
 *   e=12000           edge count
 *   pointer=0|1       enablePointerInteraction (hover hit-testing)
 *   deco=0|1          always-on labels (top-10 degree) + community hulls
 *
 * Readout: window.__bench
 */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  type ComponentType,
  type MutableRefObject,
} from 'react'
import dynamic from 'next/dynamic'
import { polygonHull } from 'd3-polygon'
import type { ForceGraphMethods, ForceGraphProps } from 'react-force-graph-2d'
import { generateBenchGraph, type BenchNode } from './benchData'

interface RenderLink {
  source: number | BenchNode
  target: number | BenchNode
  intra: boolean
}

const ForceGraph2D = dynamic(() => import('react-force-graph-2d'), {
  ssr: false,
}) as unknown as ComponentType<
  ForceGraphProps<BenchNode, RenderLink> & {
    ref?: MutableRefObject<ForceGraphMethods<BenchNode, RenderLink> | undefined>
  }
>

const HULL_COLORS = [
  '#4477aa',
  '#ee6677',
  '#228833',
  '#ccbb44',
  '#66ccee',
  '#aa3377',
  '#bbbbbb',
  '#ee8866',
]

interface BenchApi {
  firstPaintMs: number | null
  fpsCurrent: number
  fpsMin1s: number
  fpsAvgDuringTween: number
  framesTotal: number
  windows: number
  drawAvgMs: number
  drawP95Ms: number
  drawMaxMs: number
  nodeCount: number
  linkCount: number
  pointer: boolean
  deco: boolean
  devicePixelRatio: number
  reset: () => void
}

declare global {
  interface Window {
    __bench?: BenchApi
  }
}

function readParams() {
  const p =
    typeof window === 'undefined'
      ? new URLSearchParams()
      : new URLSearchParams(window.location.search)
  return {
    n: Math.max(1, Number(p.get('n') ?? 3000) || 3000),
    e: Math.max(0, Number(p.get('e') ?? 12000) || 12000),
    pointer: p.get('pointer') === '1',
    deco: p.get('deco') !== '0',
    dpr: Number(p.get('dpr') ?? 0) || 0,
    sync: p.get('sync') === '1',
  }
}

/**
 * Playwright pins the browser context to deviceScaleFactor 1, so the harness
 * cannot reproduce a Retina backing store. force-graph sizes its canvas from
 * window.devicePixelRatio at mount, so overriding it here reproduces the 2x
 * fill rate a real Retina Mac pays. Verified by reading canvas.width.
 */
function forceDevicePixelRatio(value: number) {
  if (!value || typeof window === 'undefined') return
  if (window.devicePixelRatio === value) return
  Object.defineProperty(window, 'devicePixelRatio', {
    configurable: true,
    get: () => value,
  })
}

export default function Bench() {
  const params = useMemo(() => {
    const p = readParams()
    forceDevicePixelRatio(p.dpr)
    return p
  }, [])
  const graphRef = useRef<ForceGraphMethods<BenchNode, RenderLink> | undefined>(
    undefined
  )
  const overlayRef = useRef<HTMLDivElement>(null)

  const size = useMemo(() => {
    if (typeof window === 'undefined') return { w: 1200, h: 800 }
    return { w: window.innerWidth, h: window.innerHeight - 96 }
  }, [])

  const graph = useMemo(
    () => generateBenchGraph(params.n, params.e),
    [params.n, params.e]
  )

  // graphData object identity must be stable — force-graph re-digests (and
  // re-resolves every link endpoint) whenever it changes.
  const graphData = useMemo(
    () => ({
      nodes: graph.nodes,
      links: graph.links as unknown as RenderLink[],
    }),
    [graph]
  )

  // Hull polygons are precomputed once: positions are pinned, so the hull
  // geometry is static — exactly what the real nightly-precomputed surface
  // would do. The per-frame cost measured is the FILL/STROKE, not the hull
  // solve.
  const hulls = useMemo(() => {
    if (!params.deco) return []
    return graph.hullCommunities
      .map((community, i) => {
        const pts = graph.nodes
          .filter(n => n.community === community)
          .map(n => [n.x, n.y] as [number, number])
        const hull = pts.length >= 3 ? polygonHull(pts) : null
        return hull ? { hull, color: HULL_COLORS[i % HULL_COLORS.length] } : null
      })
      .filter((h): h is { hull: [number, number][]; color: string } => !!h)
  }, [graph, params.deco])

  // ── Instrumentation ────────────────────────────────────────────────
  const syncRef = useRef(params.sync)
  syncRef.current = params.sync
  const mountedAtRef = useRef<number>(
    typeof performance !== 'undefined' ? performance.now() : 0
  )
  const statsRef = useRef({
    firstPaintMs: null as number | null,
    frames: 0,
    windowFrames: 0,
    windowStart: 0,
    fpsCurrent: 0,
    fpsMin: Number.POSITIVE_INFINITY,
    windows: 0,
    measureStart: 0,
    measureFrames: 0,
    canvasW: 0,
    canvasH: 0,
    // Main-thread draw cost: onRenderFramePre -> onRenderFramePost delta,
    // i.e. the time force-graph spends issuing this frame's draw calls on the
    // main thread. Independent of GPU pacing, and it is the number that
    // competes with React/hover work for the same thread.
    preAt: 0,
    drawSum: 0,
    drawCount: 0,
    drawMax: 0,
    drawSamples: [] as number[],
  })

  useEffect(() => {
    const api: BenchApi = {
      firstPaintMs: null,
      fpsCurrent: 0,
      fpsMin1s: 0,
      fpsAvgDuringTween: 0,
      framesTotal: 0,
      windows: 0,
      drawAvgMs: 0,
      drawP95Ms: 0,
      drawMaxMs: 0,
      nodeCount: params.n,
      linkCount: graph.links.length,
      pointer: params.pointer,
      deco: params.deco,
      devicePixelRatio:
        typeof window === 'undefined' ? 1 : window.devicePixelRatio,
      reset: () => {
        const s = statsRef.current
        const now = performance.now()
        s.fpsMin = Number.POSITIVE_INFINITY
        s.windows = 0
        s.windowFrames = 0
        s.windowStart = now
        s.measureStart = now
        s.measureFrames = 0
        s.drawSum = 0
        s.drawCount = 0
        s.drawMax = 0
        s.drawSamples = []
      },
    }
    window.__bench = api
    return () => {
      delete window.__bench
    }
  }, [params.n, params.pointer, params.deco, graph.links.length])

  const publish = useCallback(() => {
    const s = statsRef.current
    const api = window.__bench
    if (!api) return
    api.firstPaintMs = s.firstPaintMs
    api.fpsCurrent = s.fpsCurrent
    api.fpsMin1s = Number.isFinite(s.fpsMin) ? s.fpsMin : 0
    api.framesTotal = s.frames
    api.windows = s.windows
    const elapsed = performance.now() - s.measureStart
    api.fpsAvgDuringTween =
      elapsed > 0 ? Math.round((s.measureFrames / elapsed) * 1000 * 10) / 10 : 0
    const drawAvg = s.drawCount > 0 ? s.drawSum / s.drawCount : 0
    const sorted = [...s.drawSamples].sort((a, b) => a - b)
    const drawP95 =
      sorted.length > 0 ? sorted[Math.floor(sorted.length * 0.95)] ?? 0 : 0
    api.drawAvgMs = Math.round(drawAvg * 100) / 100
    api.drawP95Ms = Math.round(drawP95 * 100) / 100
    api.drawMaxMs = Math.round(s.drawMax * 100) / 100
    if (overlayRef.current) {
      overlayRef.current.textContent =
        `n=${api.nodeCount} e=${api.linkCount} ` +
        `pointer=${api.pointer ? 'ON' : 'OFF'} deco=${api.deco ? 'ON' : 'OFF'} ` +
        `sync=${syncRef.current ? 'ON' : 'OFF'} ` +
        `dpr=${api.devicePixelRatio} backing=${s.canvasW}x${s.canvasH} | ` +
        `firstPaint=${api.firstPaintMs == null ? '-' : api.firstPaintMs.toFixed(1)}ms | ` +
        `fps cur=${api.fpsCurrent} min=${api.fpsMin1s} ` +
        `avg=${api.fpsAvgDuringTween} (${api.windows} windows) | ` +
        `draw avg=${drawAvg.toFixed(2)}ms p95=${drawP95.toFixed(2)}ms max=${s.drawMax.toFixed(2)}ms`
    }
  }, [])

  // Frame accounting lives in onRenderFramePost: it fires once per canvas
  // render, so it measures the RENDER rate. A bare rAF counter would report
  // the browser's 60Hz callback rate even when the graph skipped a frame.
  const handleRenderFramePre = useCallback(() => {
    statsRef.current.preAt = performance.now()
  }, [])

  const handleRenderFramePost = useCallback(
    (ctx: CanvasRenderingContext2D) => {
      const s = statsRef.current

      // Decoration pass first, so the draw-cost sample below covers the WHOLE
      // frame (nodes + links + labels + hulls), not just the library's part.
      if (hulls.length > 0) {
        ctx.save()
        for (const { hull, color } of hulls) {
          ctx.beginPath()
          ctx.moveTo(hull[0][0], hull[0][1])
          for (let i = 1; i < hull.length; i++)
            ctx.lineTo(hull[i][0], hull[i][1])
          ctx.closePath()
          ctx.fillStyle = color
          ctx.globalAlpha = 0.1
          ctx.fill()
          ctx.globalAlpha = 0.35
          ctx.strokeStyle = color
          ctx.lineWidth = 2
          ctx.stroke()
        }
        ctx.restore()
      }

      // sync=1 exists only to demonstrate that it is NOT a usable probe:
      // frequent getImageData makes Chrome fall back to a software canvas, so
      // it measures a different renderer. Left in, off by default.
      if (syncRef.current) ctx.getImageData(0, 0, 1, 1)

      const now = performance.now()

      if (s.firstPaintMs == null) {
        s.firstPaintMs = now - mountedAtRef.current
        s.canvasW = ctx.canvas.width
        s.canvasH = ctx.canvas.height
        s.windowStart = now
        s.measureStart = now
      }

      if (s.preAt > 0) {
        const draw = now - s.preAt
        s.drawSum += draw
        s.drawCount++
        if (draw > s.drawMax) s.drawMax = draw
        if (s.drawSamples.length < 5000) s.drawSamples.push(draw)
      }

      s.frames++
      s.windowFrames++
      s.measureFrames++

      if (now - s.windowStart >= 1000) {
        const fps = Math.round((s.windowFrames / (now - s.windowStart)) * 1000)
        s.fpsCurrent = fps
        s.windows++
        // Skip the very first window: it includes the first-paint frame and
        // the pre-tween idle, which is not a pan/zoom measurement.
        if (s.windows > 1 && fps < s.fpsMin) s.fpsMin = fps
        s.windowFrames = 0
        s.windowStart = now
        publish()
      }
    },
    [hulls, publish]
  )

  const labeledIds = graph.labeledIds
  const nodeCanvasObject = useCallback(
    (node: BenchNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const x = node.x ?? 0
      const y = node.y ?? 0
      ctx.beginPath()
      ctx.arc(x, y, 4, 0, Math.PI * 2)
      ctx.fillStyle = HULL_COLORS[node.community % HULL_COLORS.length]
      ctx.fill()
      if (!labeledIds.has(node.id)) return
      const fontSize = Math.max(10 / globalScale, 3)
      ctx.font = `600 ${fontSize}px sans-serif`
      ctx.textAlign = 'center'
      ctx.textBaseline = 'bottom'
      ctx.fillStyle = 'rgba(20,20,20,0.92)'
      ctx.fillText(node.name, x, y - 6)
    },
    [labeledIds]
  )

  const nodePointerAreaPaint = useCallback(
    (node: BenchNode, color: string, ctx: CanvasRenderingContext2D) => {
      ctx.beginPath()
      ctx.arc(node.x ?? 0, node.y ?? 0, 6, 0, Math.PI * 2)
      ctx.fillStyle = color
      ctx.fill()
    },
    []
  )

  const linkColor = useCallback(
    (link: RenderLink) =>
      link.intra ? 'rgba(90,90,110,0.30)' : 'rgba(180,90,90,0.28)',
    []
  )
  const linkWidth = useCallback(() => 1, [])

  // ── Programmatic camera tween loop ─────────────────────────────────
  // Synthetic mouse events never reach the canvas (known project pattern),
  // so pan/zoom is driven through the force-graph ref API.
  useEffect(() => {
    let cancelled = false
    const timers: number[] = []

    const start = window.setTimeout(() => {
      const fg = graphRef.current
      if (!fg || cancelled) return
      fg.zoomToFit(0, 40)
      const base = fg.zoom()
      const center = fg.centerAt()
      const span = 900 / Math.max(base, 0.01)

      const steps: { at: number; run: () => void }[] = [
        { at: 0, run: () => fg.zoom(base * 0.5, 2400) },
        { at: 2400, run: () => fg.centerAt(center.x + span, center.y + span * 0.6, 2400) },
        { at: 4800, run: () => fg.centerAt(center.x - span, center.y - span * 0.6, 2400) },
        { at: 7200, run: () => fg.zoom(base * 2.5, 2400) },
        {
          at: 9600,
          run: () => {
            fg.zoom(base, 1200)
            fg.centerAt(center.x, center.y, 1200)
          },
        },
      ]
      const CYCLE = 10800

      const schedule = () => {
        if (cancelled) return
        for (const step of steps) {
          timers.push(window.setTimeout(step.run, step.at))
        }
        timers.push(window.setTimeout(schedule, CYCLE))
      }
      // Zero the min-FPS baseline once motion actually begins.
      window.__bench?.reset()
      schedule()
    }, 1500)

    return () => {
      cancelled = true
      window.clearTimeout(start)
      for (const t of timers) window.clearTimeout(t)
    }
  }, [])

  return (
    <div style={{ background: '#f6f4ef' }}>
      <div
        id="bench-overlay"
        ref={overlayRef}
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          zIndex: 50,
          padding: '10px 14px',
          font: '13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace',
          background: '#111',
          color: '#0f0',
          whiteSpace: 'pre-wrap',
        }}
      >
        warming up...
      </div>
      <div style={{ paddingTop: 44 }}>
        <ForceGraph2D
          ref={graphRef}
          graphData={
            graphData as unknown as React.ComponentProps<
              typeof ForceGraph2D
            >['graphData']
          }
          width={size.w}
          height={size.h}
          nodeId="id"
          nodeCanvasObject={nodeCanvasObject}
          nodePointerAreaPaint={nodePointerAreaPaint}
          linkSource="source"
          linkTarget="target"
          linkColor={linkColor}
          linkWidth={linkWidth}
          onRenderFramePre={handleRenderFramePre}
          onRenderFramePost={handleRenderFramePost}
          warmupTicks={0}
          cooldownTicks={0}
          enableNodeDrag={false}
          enablePointerInteraction={params.pointer}
          // Deliberate: force a continuous repaint so the FPS number is an
          // unambiguous sustained render rate rather than a mix of rendered
          // and skipped frames.
          autoPauseRedraw={false}
          minZoom={0.01}
          maxZoom={30}
          backgroundColor="#f6f4ef"
        />
      </div>
    </div>
  )
}
