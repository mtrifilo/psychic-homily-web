import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { GraphAccessibleTree, type GraphAccessibleTreeProps } from './GraphAccessibleTree'
import type { AccessibleTreeGraphNode, AccessibleTreeRow } from './graphTreeModel'

type Row = AccessibleTreeRow<AccessibleTreeGraphNode>

const node = (id: number, name: string): AccessibleTreeGraphNode => ({ id, name, slug: name.toLowerCase() })

// Dehd (expanded) → Opener A child → Lifeguard → Horsegirl
const rows: Row[] = [
  { node: node(2, 'Dehd'), level: 1, expanded: true, expanding: false, posInSet: 1, setSize: 3 },
  { node: node(5, 'Opener A'), level: 2, expanded: false, expanding: false, posInSet: 1, setSize: 1 },
  { node: node(3, 'Lifeguard'), level: 1, expanded: false, expanding: false, posInSet: 2, setSize: 3 },
  { node: node(4, 'Horsegirl'), level: 1, expanded: false, expanding: false, posInSet: 3, setSize: 3 },
]

function renderTree(overrides: Partial<GraphAccessibleTreeProps<AccessibleTreeGraphNode>> = {}) {
  const onToggleExpand = vi.fn()
  render(
    <GraphAccessibleTree
      rows={rows}
      label="Connections for Faetooth"
      onToggleExpand={onToggleExpand}
      {...overrides}
    />,
  )
  return { onToggleExpand }
}

const items = () => screen.getAllByRole('treeitem')

describe('GraphAccessibleTree', () => {
  it('renders a tree with per-row ARIA structure', () => {
    renderTree()
    const tree = screen.getByRole('tree', { name: 'Connections for Faetooth' })
    expect(tree).toBeInTheDocument()
    const dehd = screen.getByRole('treeitem', { name: /Dehd/ })
    expect(dehd).toHaveAttribute('aria-expanded', 'true')
    expect(dehd).toHaveAttribute('aria-level', '1')
    expect(dehd).toHaveAttribute('aria-setsize', '3')
    expect(dehd).toHaveAttribute('aria-posinset', '1')
    const openerA = screen.getByRole('treeitem', { name: /Opener A/ })
    expect(openerA).toHaveAttribute('aria-level', '2')
    expect(openerA).toHaveAttribute('aria-expanded', 'false')
  })

  it('has exactly one roving tabstop (first row), rest at -1', () => {
    renderTree()
    const all = items()
    expect(all[0]).toHaveAttribute('tabindex', '0')
    expect(all.slice(1).every(el => el.getAttribute('tabindex') === '-1')).toBe(true)
  })

  it('ArrowDown / ArrowUp move the tabstop and DOM focus', () => {
    renderTree()
    const tree = screen.getByRole('tree')
    fireEvent.keyDown(tree, { key: 'ArrowDown' })
    expect(items()[1]).toHaveAttribute('tabindex', '0')
    expect(items()[1]).toHaveFocus()
    fireEvent.keyDown(tree, { key: 'ArrowUp' })
    expect(items()[0]).toHaveAttribute('tabindex', '0')
    expect(items()[0]).toHaveFocus()
  })

  it('Home / End jump to the first / last row', () => {
    renderTree()
    const tree = screen.getByRole('tree')
    fireEvent.keyDown(tree, { key: 'End' })
    expect(items()[3]).toHaveFocus()
    fireEvent.keyDown(tree, { key: 'Home' })
    expect(items()[0]).toHaveFocus()
  })

  it('Enter and Space toggle expand on the focused row', () => {
    const { onToggleExpand } = renderTree()
    const tree = screen.getByRole('tree')
    fireEvent.keyDown(tree, { key: 'Enter' })
    expect(onToggleExpand).toHaveBeenCalledWith(expect.objectContaining({ id: 2 }))
    fireEvent.keyDown(tree, { key: ' ' })
    expect(onToggleExpand).toHaveBeenCalledTimes(2)
  })

  it('ArrowRight expands a collapsed row; on an expanded row it steps into the first child', () => {
    const { onToggleExpand } = renderTree()
    const tree = screen.getByRole('tree')
    // Focus starts on Dehd (expanded) → ArrowRight steps into Opener A, no toggle.
    fireEvent.keyDown(tree, { key: 'ArrowRight' })
    expect(items()[1]).toHaveFocus()
    expect(onToggleExpand).not.toHaveBeenCalled()
    // Opener A is collapsed → ArrowRight expands it (toggle).
    fireEvent.keyDown(tree, { key: 'ArrowRight' })
    expect(onToggleExpand).toHaveBeenCalledWith(expect.objectContaining({ id: 5 }))
  })

  it('ArrowLeft collapses an expanded row; on a child it moves to the parent', () => {
    const { onToggleExpand } = renderTree()
    const tree = screen.getByRole('tree')
    // Move to the child (Opener A, level 2), then ArrowLeft → back to parent Dehd.
    fireEvent.keyDown(tree, { key: 'ArrowDown' })
    expect(items()[1]).toHaveFocus()
    fireEvent.keyDown(tree, { key: 'ArrowLeft' })
    expect(items()[0]).toHaveFocus()
    expect(onToggleExpand).not.toHaveBeenCalled()
    // Now on Dehd (expanded) → ArrowLeft collapses (toggle).
    fireEvent.keyDown(tree, { key: 'ArrowLeft' })
    expect(onToggleExpand).toHaveBeenCalledWith(expect.objectContaining({ id: 2 }))
  })

  it('click toggles expand and focuses the row', () => {
    const { onToggleExpand } = renderTree()
    fireEvent.click(screen.getByRole('treeitem', { name: /Lifeguard/ }))
    expect(onToggleExpand).toHaveBeenCalledWith(expect.objectContaining({ id: 3 }))
    expect(screen.getByRole('treeitem', { name: /Lifeguard/ })).toHaveFocus()
  })

  it('renders the empty state (no tree) when there are no rows', () => {
    renderTree({ rows: [], emptyLabel: 'No connections to navigate.' })
    expect(screen.queryByRole('tree')).not.toBeInTheDocument()
    expect(screen.getByText('No connections to navigate.')).toBeInTheDocument()
  })

  it('marks an in-flight expand with aria-busy', () => {
    renderTree({
      rows: [{ node: node(2, 'Dehd'), level: 1, expanded: false, expanding: true, posInSet: 1, setSize: 1 }],
    })
    expect(screen.getByRole('treeitem', { name: /Dehd/ })).toHaveAttribute('aria-busy', 'true')
  })
})
