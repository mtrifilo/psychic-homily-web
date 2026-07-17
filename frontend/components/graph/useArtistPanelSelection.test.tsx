import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { GraphNode } from './ForceGraphView'
import { useArtistPanelSelection } from './useArtistPanelSelection'

// Unit tests for the shared node-select → context-panel wiring (PSY-1451).
// The consumer suites (HomeSceneGraph / SceneGraphVisualization /
// StationGraphVisualization) cover the integration with ForceGraphView and
// the real ArtistContextPanel; this file pins the hook's own contract with a
// minimal harness — in particular the guarded focus return that fixes the
// PR #1562 deferred finding (Esc while focus sits on the accessible node
// list must not yank focus off that list).

const nodeA: GraphNode = { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 0 }
const nodeB: GraphNode = { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0 }

function Harness({ nodes }: { nodes: GraphNode[] }) {
  const {
    selectedNode,
    canvasWrapRef,
    panelRef,
    handleNodeClick,
    handleBackgroundClick,
    handlePanelClose,
    handleConnectionInspectOpen,
    clearSelection,
  } = useArtistPanelSelection({
    resolveNode: selected => nodes.find(n => n.id === selected.id) ?? null,
  })

  return (
    <div ref={canvasWrapRef} tabIndex={-1} data-testid="wrap">
      <button type="button" onClick={() => handleNodeClick(nodeA)}>
        node-alpha
      </button>
      <button type="button" onClick={() => handleNodeClick(nodeB)}>
        node-beta
      </button>
      <button type="button" onClick={handleBackgroundClick}>
        background
      </button>
      <button type="button" onClick={handleConnectionInspectOpen}>
        edge-inspect
      </button>
      <button type="button" onClick={clearSelection}>
        clear
      </button>
      {/* Stands in for ForceGraphView's accessible node list — inside the
          wrap, outside the panel. */}
      <button type="button" data-testid="node-list-item">
        list-item
      </button>
      {selectedNode && (
        <section
          ref={panelRef}
          tabIndex={-1}
          aria-label={`About ${selectedNode.name}`}
        >
          <button type="button" onClick={handlePanelClose}>
            close
          </button>
        </section>
      )}
    </div>
  )
}

function panel(name: string) {
  return screen.queryByRole('region', { name: `About ${name}` })
}

describe('useArtistPanelSelection', () => {
  it('click selects; clicking another node switches; second click on the same node deselects', () => {
    render(<Harness nodes={[nodeA, nodeB]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    expect(panel('Alpha')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'node-beta' }))
    expect(panel('Alpha')).toBeNull()
    expect(panel('Beta')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'node-beta' }))
    expect(panel('Beta')).toBeNull()
  })

  it('background click closes an open panel; with no panel open it is a no-op (no focus steal)', () => {
    render(<Harness nodes={[nodeA]} />)
    // No selection: no-op, and the wrap must NOT grab focus.
    fireEvent.click(screen.getByRole('button', { name: 'background' }))
    expect(document.activeElement).not.toBe(screen.getByTestId('wrap'))

    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    fireEvent.click(screen.getByRole('button', { name: 'background' }))
    expect(panel('Alpha')).toBeNull()
    expect(document.activeElement).toBe(screen.getByTestId('wrap'))
  })

  it('close returns focus to the canvas wrap when focus was inside the unmounting panel', () => {
    render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    screen.getByRole('button', { name: 'close' }).focus()
    fireEvent.click(screen.getByRole('button', { name: 'close' }))
    expect(panel('Alpha')).toBeNull()
    expect(document.activeElement).toBe(screen.getByTestId('wrap'))
  })

  it('close returns focus to the canvas wrap when focus was already lost to body (PSY-1313)', () => {
    render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    expect(document.activeElement).toBe(document.body)
    fireEvent.click(screen.getByRole('button', { name: 'close' }))
    expect(document.activeElement).toBe(screen.getByTestId('wrap'))
  })

  it('close does NOT yank focus when it sits outside the panel, e.g. on the accessible node list (#1562 deferred fix)', () => {
    render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    const listItem = screen.getByTestId('node-list-item')
    listItem.focus()
    // Esc reaches the hook as handlePanelClose (via the panel's
    // DismissableLayer) without moving focus first — simulate by invoking
    // close while focus is on the list item.
    fireEvent.click(screen.getByRole('button', { name: 'close' }))
    expect(panel('Alpha')).toBeNull()
    expect(document.activeElement).toBe(listItem)
  })

  it('edge-inspect open deselects without moving focus', () => {
    render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    const listItem = screen.getByTestId('node-list-item')
    listItem.focus()
    fireEvent.click(screen.getByRole('button', { name: 'edge-inspect' }))
    expect(panel('Alpha')).toBeNull()
    expect(document.activeElement).toBe(listItem)
  })

  it('clearSelection closes the panel without moving focus', () => {
    render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    const listItem = screen.getByTestId('node-list-item')
    listItem.focus()
    fireEvent.click(screen.getByRole('button', { name: 'clear' }))
    expect(panel('Alpha')).toBeNull()
    expect(document.activeElement).toBe(listItem)
  })

  it('clears the selection during render when the resolver drops the node', () => {
    const { rerender } = render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    expect(panel('Alpha')).toBeInTheDocument()

    rerender(<Harness nodes={[]} />)
    expect(panel('Alpha')).toBeNull()
  })

  it('a dropped-then-restored node stays deselected (the clear is real, not a render-time mask)', () => {
    const { rerender } = render(<Harness nodes={[nodeA]} />)
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    rerender(<Harness nodes={[]} />)
    rerender(<Harness nodes={[nodeA]} />)
    expect(panel('Alpha')).toBeNull()
  })
})
