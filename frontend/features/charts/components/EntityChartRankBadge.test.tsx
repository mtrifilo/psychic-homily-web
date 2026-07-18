import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

import { EntityChartRankBadge } from './EntityChartRankBadge'

describe('EntityChartRankBadge', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('renders rank + window into module drill-down when ranked', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entity_type: 'show',
      entity_id: 12,
      window: 'quarter',
      module: 'most-anticipated',
      rank: 3,
    })

    render(<EntityChartRankBadge entityType="show" entityId={12} />, {
      wrapper: createWrapper(),
    })

    const link = await screen.findByRole('link', {
      name: /No\. 3.*most-saved upcoming show — this quarter →/,
    })
    expect(link).toHaveAttribute(
      'href',
      '/charts/most-anticipated?window=quarter'
    )
    expect(screen.getByText('Charts')).toBeInTheDocument()
  })

  it('renders nothing when rank is null', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entity_type: 'artist',
      entity_id: 7,
      window: 'quarter',
      module: 'most-active-artists',
      rank: null,
    })

    const { container } = render(
      <EntityChartRankBadge entityType="artist" entityId={7} />,
      { wrapper: createWrapper() }
    )

    await vi.waitFor(() => expect(mockApiRequest).toHaveBeenCalled())
    expect(
      screen.queryByTestId('entity-chart-rank-badge')
    ).not.toBeInTheDocument()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing while loading (non-blocking)', () => {
    mockApiRequest.mockReturnValue(new Promise(() => {}))

    const { container } = render(
      <EntityChartRankBadge entityType="show" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(
      screen.queryByTestId('entity-chart-rank-badge')
    ).not.toBeInTheDocument()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing on fetch error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('network'))

    const { container } = render(
      <EntityChartRankBadge entityType="venue" entityId={9} />,
      { wrapper: createWrapper() }
    )

    await vi.waitFor(() => expect(mockApiRequest).toHaveBeenCalled())
    expect(
      screen.queryByTestId('entity-chart-rank-badge')
    ).not.toBeInTheDocument()
    expect(container).toBeEmptyDOMElement()
  })

  it('uses artist module copy from Figma', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entity_type: 'artist',
      entity_id: 42,
      window: 'quarter',
      module: 'most-active-artists',
      rank: 7,
    })

    render(<EntityChartRankBadge entityType="artist" entityId={42} />, {
      wrapper: createWrapper(),
    })

    expect(
      await screen.findByRole('link', {
        name: /No\. 7.*hardest-working artists — this quarter →/,
      })
    ).toHaveAttribute(
      'href',
      '/charts/most-active-artists?window=quarter'
    )
  })
})
