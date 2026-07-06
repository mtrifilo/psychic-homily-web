import { describe, it, expect } from 'vitest'
import {
  buildGraphTree,
  flattenVisibleTree,
  type AccessibleTreeGraph,
  type AccessibleTreeGraphNode,
} from './graphTreeModel'

const n = (id: number, name: string): AccessibleTreeGraphNode => ({
  id,
  name,
  slug: name.toLowerCase(),
})

const base: AccessibleTreeGraph<AccessibleTreeGraphNode> = {
  center: { id: 1 },
  nodes: [n(2, 'Dehd'), n(3, 'Lifeguard'), n(4, 'Horsegirl')],
}

describe('buildGraphTree', () => {
  it('lists the center neighbours as level-1 items, sorted by name when unranked', () => {
    const tree = buildGraphTree(base, new Map(), new Set())
    expect(tree.map(t => t.node.name)).toEqual(['Dehd', 'Horsegirl', 'Lifeguard'])
    expect(tree.every(t => t.level === 1 && !t.expanded && t.children.length === 0)).toBe(true)
  })

  it('orders by rank (desc) when supplied, name-tiebroken', () => {
    const rank = new Map([
      [2, 0.1],
      [3, 0.9],
      [4, 0.5],
    ])
    const tree = buildGraphTree(base, new Map(), new Set(), rank)
    expect(tree.map(t => t.node.name)).toEqual(['Lifeguard', 'Horsegirl', 'Dehd'])
  })

  it('reveals an expanded node’s neighbours as nested children', () => {
    const expansions = new Map<number, AccessibleTreeGraph<AccessibleTreeGraphNode>>([
      [2, { center: { id: 2 }, nodes: [n(5, 'Opener A'), n(6, 'Opener B')] }],
    ])
    const tree = buildGraphTree(base, expansions, new Set())
    const dehd = tree.find(t => t.node.id === 2)!
    expect(dehd.expanded).toBe(true)
    expect(dehd.children.map(c => c.node.name)).toEqual(['Opener A', 'Opener B'])
    expect(dehd.children.every(c => c.level === 2)).toBe(true)
  })

  it('dedups: a base neighbour revealed again by an expansion stays a level-1 root', () => {
    // Expanding Dehd(2) reveals Lifeguard(3), which is already a base neighbour.
    const expansions = new Map<number, AccessibleTreeGraph<AccessibleTreeGraphNode>>([
      [2, { center: { id: 2 }, nodes: [n(3, 'Lifeguard'), n(5, 'Opener A')] }],
    ])
    const tree = buildGraphTree(base, expansions, new Set())
    // Lifeguard stays a root, NOT a child of Dehd.
    expect(tree.filter(t => t.node.id === 3)).toHaveLength(1)
    const dehd = tree.find(t => t.node.id === 2)!
    expect(dehd.children.map(c => c.node.id)).toEqual([5]) // only the genuinely new node
  })

  it('dedups across two expanded parents: a shared grandchild shows under the first only', () => {
    const expansions = new Map<number, AccessibleTreeGraph<AccessibleTreeGraphNode>>([
      [2, { center: { id: 2 }, nodes: [n(7, 'Shared')] }],
      [3, { center: { id: 3 }, nodes: [n(7, 'Shared')] }],
    ])
    // Rank so Dehd(2) sorts before Lifeguard(3).
    const rank = new Map([[2, 1], [3, 0.5], [4, 0.4]])
    const tree = buildGraphTree(base, expansions, new Set(), rank)
    const dehd = tree.find(t => t.node.id === 2)!
    const lifeguard = tree.find(t => t.node.id === 3)!
    expect(dehd.children.map(c => c.node.id)).toEqual([7])
    expect(lifeguard.children).toHaveLength(0)
  })

  it('marks in-flight expands', () => {
    const tree = buildGraphTree(base, new Map(), new Set([3]))
    expect(tree.find(t => t.node.id === 3)!.expanding).toBe(true)
    expect(tree.find(t => t.node.id === 2)!.expanding).toBe(false)
  })
})

describe('flattenVisibleTree', () => {
  it('emits only visible rows with posInSet/setSize and skips collapsed subtrees', () => {
    const expansions = new Map<number, AccessibleTreeGraph<AccessibleTreeGraphNode>>([
      [2, { center: { id: 2 }, nodes: [n(5, 'Opener A'), n(6, 'Opener B')] }],
    ])
    const rank = new Map([[2, 1], [3, 0.5], [4, 0.4]])
    const rows = flattenVisibleTree(buildGraphTree(base, expansions, new Set(), rank))
    // Dehd(expanded) → its 2 children → Lifeguard → Horsegirl
    expect(rows.map(r => r.node.name)).toEqual([
      'Dehd',
      'Opener A',
      'Opener B',
      'Lifeguard',
      'Horsegirl',
    ])
    const dehd = rows[0]
    expect(dehd).toMatchObject({ level: 1, posInSet: 1, setSize: 3, expanded: true })
    const openerA = rows[1]
    expect(openerA).toMatchObject({ level: 2, posInSet: 1, setSize: 2 })
    const lifeguard = rows[3]
    expect(lifeguard).toMatchObject({ level: 1, posInSet: 2, setSize: 3 })
  })
})
