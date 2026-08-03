/**
 * PSY-1724 spike — synthetic precomputed-layout graph generator.
 *
 * THROWAWAY BENCH CODE. Not product code, never merged.
 *
 * Shape mirrors the planned "Map of the Scene" payload: nightly-precomputed
 * node positions (so d3-force does zero work — every node is pinned via
 * fx/fy), community assignment, mostly-intra-community edges.
 */

export interface BenchNode {
  id: number
  name: string
  community: number
  x: number
  y: number
  fx: number
  fy: number
  degree: number
}

export interface BenchLink {
  source: number
  target: number
  intra: boolean
}

/** Mulberry32 — small, fast, deterministic. */
function makePrng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** Box-Muller gaussian. */
function gaussian(rand: () => number): number {
  const u = Math.max(rand(), 1e-9)
  const v = rand()
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v)
}

const COMMUNITY_COUNT = 40
/** Radius of the ring the community centers sit on (graph coords). */
const MACRO_RADIUS = 1800
/** Gaussian scatter sigma inside a community. */
const SCATTER_SIGMA = 110
/** Share of edges that stay inside a community. */
const INTRA_SHARE = 0.85

export interface BenchGraph {
  nodes: BenchNode[]
  links: BenchLink[]
  /** Node ids of the top-degree nodes that get always-on labels. */
  labeledIds: Set<number>
  /** Community ids of the largest communities that get hulls. */
  hullCommunities: number[]
}

export function generateBenchGraph(
  nodeCount: number,
  linkCount: number,
  seed = 1724
): BenchGraph {
  const rand = makePrng(seed)

  // Community centers: two concentric rings + a little jitter, so hull
  // polygons overlap the way real Leiden communities do.
  const centers: { x: number; y: number }[] = []
  for (let c = 0; c < COMMUNITY_COUNT; c++) {
    const ring = c % 2 === 0 ? 1 : 0.55
    const angle = (c / COMMUNITY_COUNT) * Math.PI * 2 * 3
    centers.push({
      x: Math.cos(angle) * MACRO_RADIUS * ring + (rand() - 0.5) * 260,
      y: Math.sin(angle) * MACRO_RADIUS * ring + (rand() - 0.5) * 260,
    })
  }

  // Skewed community sizes (a few big scenes, a long tail) via a
  // Zipf-ish weight, so hull areas differ like real data.
  const weights: number[] = []
  let weightTotal = 0
  for (let c = 0; c < COMMUNITY_COUNT; c++) {
    const w = 1 / Math.pow(c + 1, 0.6)
    weights.push(w)
    weightTotal += w
  }

  const nodes: BenchNode[] = []
  const membersByCommunity: number[][] = Array.from(
    { length: COMMUNITY_COUNT },
    () => []
  )

  for (let i = 0; i < nodeCount; i++) {
    // Weighted community pick.
    let r = rand() * weightTotal
    let community = COMMUNITY_COUNT - 1
    for (let c = 0; c < COMMUNITY_COUNT; c++) {
      r -= weights[c]
      if (r <= 0) {
        community = c
        break
      }
    }
    const center = centers[community]
    const x = center.x + gaussian(rand) * SCATTER_SIGMA
    const y = center.y + gaussian(rand) * SCATTER_SIGMA
    nodes.push({
      id: i,
      name: `Artist ${i}`,
      community,
      x,
      y,
      fx: x,
      fy: y,
      degree: 0,
    })
    membersByCommunity[community].push(i)
  }

  const links: BenchLink[] = []
  const seen = new Set<string>()
  let guard = 0
  const guardLimit = linkCount * 12

  while (links.length < linkCount && guard++ < guardLimit) {
    let s: number
    let t: number
    const intra = rand() < INTRA_SHARE
    if (intra) {
      const community = Math.floor(rand() * COMMUNITY_COUNT)
      const members = membersByCommunity[community]
      if (members.length < 2) continue
      s = members[Math.floor(rand() * members.length)]
      t = members[Math.floor(rand() * members.length)]
    } else {
      s = Math.floor(rand() * nodeCount)
      t = Math.floor(rand() * nodeCount)
    }
    if (s === t) continue
    const key = s < t ? `${s}:${t}` : `${t}:${s}`
    if (seen.has(key)) continue
    seen.add(key)
    links.push({ source: s, target: t, intra })
    nodes[s].degree++
    nodes[t].degree++
  }

  const byDegree = [...nodes].sort((a, b) => b.degree - a.degree)
  const labeledIds = new Set(byDegree.slice(0, 10).map(n => n.id))

  const sizes = membersByCommunity.map((m, c) => ({ c, size: m.length }))
  sizes.sort((a, b) => b.size - a.size)
  const hullCommunities = sizes.slice(0, 8).map(s => s.c)

  return { nodes, links, labeledIds, hullCommunities }
}
