import { describe, it, expect, vi } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

import { ConnectionPanel, type PanelConnection } from './ConnectionPanel'
import { buildLinkLabelText } from './edgeGrammar'

const source = { name: 'Dehd', slug: 'dehd' }
const target = { name: 'Lifeguard', slug: 'lifeguard' }

const connections: PanelConnection[] = [
  { type: 'shared_bills', score: 0.3, detail: { shared_count: 3, last_shared: '2026-05-14' } },
  { type: 'shared_label', detail: { shared_count: 1, label_names: 'Fire Talk' } },
  { type: 'similar', score: 0.82, votes_up: 4, votes_down: 1 },
]

const renderPanel = (overrides: Partial<React.ComponentProps<typeof ConnectionPanel>> = {}) => {
  const onClose = vi.fn()
  renderWithProviders(
    <ConnectionPanel
      source={source}
      target={target}
      connections={connections}
      onClose={onClose}
      {...overrides}
    />,
  )
  return { onClose }
}

describe('ConnectionPanel', () => {
  it('renders one row per connection with the SAME copy the canvas tooltip uses', () => {
    renderPanel()
    // Copy parity is the contract: the panel and tooltip share
    // buildLinkLabelText, so assert through it rather than duplicating
    // format strings that could drift.
    for (const conn of connections) {
      expect(screen.getByText(buildLinkLabelText(conn))).toBeInTheDocument()
    }
    expect(screen.getByText('Shared Bills')).toBeInTheDocument()
    expect(screen.getByText('Shared Label')).toBeInTheDocument()
  })

  it('links the endpoint names to their artist pages', () => {
    renderPanel()
    expect(screen.getByRole('link', { name: 'Dehd' })).toHaveAttribute('href', '/artists/dehd')
    expect(screen.getByRole('link', { name: 'Lifeguard' })).toHaveAttribute(
      'href',
      '/artists/lifeguard',
    )
  })

  it('renders a slug-less endpoint as plain text (no dead link)', () => {
    renderPanel({ source: { name: 'Dehd' } })
    expect(screen.queryByRole('link', { name: 'Dehd' })).not.toBeInTheDocument()
    expect(screen.getByText('Dehd')).toBeInTheDocument()
  })

  it('renders phase-2 entities as links when present (PSY-1335 contract)', () => {
    renderPanel({
      connections: [
        {
          type: 'shared_label',
          detail: { shared_count: 1, label_names: 'Fire Talk' },
          entities: [{ kind: 'label', id: 7, slug: 'fire-talk', name: 'Fire Talk' }],
        },
        {
          type: 'shared_bills',
          detail: { shared_count: 1 },
          entities: [
            { kind: 'show', id: 9, slug: 'dehd-empty-bottle', name: 'Empty Bottle', date: '2026-05-14' },
          ],
        },
      ],
    })
    expect(screen.getByRole('link', { name: 'Fire Talk' })).toHaveAttribute(
      'href',
      '/labels/fire-talk',
    )
    expect(screen.getByRole('link', { name: '2026-05-14 · Empty Bottle' })).toHaveAttribute(
      'href',
      '/shows/dehd-empty-bottle',
    )
  })

  it('closes via the close button and via Escape', () => {
    const { onClose } = renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'Close connection details' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('renders nothing for an empty connection list', () => {
    renderPanel({ connections: [] })
    expect(screen.queryByRole('region')).not.toBeInTheDocument()
  })

  it('renders community-contributed names as text, never markup (XSS)', () => {
    renderPanel({
      connections: [
        { type: 'shared_label', detail: { shared_count: 1, label_names: '<img src=x onerror=alert(1)>' } },
      ],
    })
    // React text rendering escapes at the sink: the string is visible
    // verbatim and no <img> element exists in the document.
    expect(screen.getByText(/Both on <img src=x onerror=alert\(1\)>/)).toBeInTheDocument()
    expect(document.querySelector('img')).toBeNull()
  })
})
