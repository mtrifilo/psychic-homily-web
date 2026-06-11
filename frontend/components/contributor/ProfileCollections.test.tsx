import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ProfileCollections } from './ProfileCollections'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

const mockUseUserPublicCollections = vi.fn()

vi.mock('@/features/collections', () => ({
  useUserPublicCollections: (username: string) =>
    mockUseUserPublicCollections(username),
}))

function makeCollection(id: number, overrides: Record<string, unknown> = {}) {
  return {
    id,
    title: `Collection ${id}`,
    slug: `collection-${id}`,
    item_count: id * 2,
    like_count: id,
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ProfileCollections', () => {
  it('renders dense rows with title links and mono items/likes counts', () => {
    mockUseUserPublicCollections.mockReturnValue({
      data: {
        collections: [
          makeCollection(1, {
            title: 'Best of AZ DIY 2026',
            slug: 'best-of-az-diy-2026',
            item_count: 24,
            like_count: 31,
          }),
        ],
        total: 1,
      },
    })

    renderWithProviders(<ProfileCollections username="alice" isOwner={false} />)
    const link = screen.getByRole('link', { name: 'Best of AZ DIY 2026' })
    expect(link).toHaveAttribute('href', '/collections/best-of-az-diy-2026')
    expect(screen.getByText(/24 items · 31 likes/)).toBeInTheDocument()
  })

  it('singularizes one item / one like', () => {
    mockUseUserPublicCollections.mockReturnValue({
      data: {
        collections: [makeCollection(1, { item_count: 1, like_count: 1 })],
        total: 1,
      },
    })

    renderWithProviders(<ProfileCollections username="alice" isOwner={false} />)
    expect(screen.getByText(/1 item · 1 like/)).toBeInTheDocument()
  })

  it('caps at five rows and expands in place via "View all"', () => {
    const collections = [1, 2, 3, 4, 5, 6, 7].map(id => makeCollection(id))
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections, total: 7 },
    })

    renderWithProviders(<ProfileCollections username="alice" isOwner={false} />)
    expect(screen.getAllByRole('link')).toHaveLength(5)
    expect(screen.queryByText('Collection 6')).not.toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('button', { name: /view all 7 collections/i })
    )
    expect(screen.getAllByRole('link')).toHaveLength(7)
    expect(
      screen.queryByRole('button', { name: /view all/i })
    ).not.toBeInTheDocument()
  })

  it('shows a residual overflow line after expanding when total exceeds the fetched page', () => {
    const collections = Array.from({ length: 20 }, (_, i) =>
      makeCollection(i + 1)
    )
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections, total: 25 },
    })

    renderWithProviders(<ProfileCollections username="alice" isOwner={false} />)
    expect(screen.queryByText(/more/)).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: /view all 25 collections/i })
    )
    expect(screen.getByText(/\+ 5 more/)).toBeInTheDocument()
  })

  it('renders nothing for a visitor when there are no collections', () => {
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections: [], total: 0 },
    })

    const { container } = renderWithProviders(
      <ProfileCollections username="alice" isOwner={false} />
    )
    expect(container.innerHTML).toBe('')
  })

  it('shows the owner a start-a-collection prompt when empty', () => {
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections: [], total: 0 },
    })

    renderWithProviders(<ProfileCollections username="alice" isOwner />)
    expect(screen.getByText(/Curate a list worth sharing/)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /start a collection/i })
    ).toHaveAttribute('href', '/collections')
  })

  it('renders nothing while loading', () => {
    mockUseUserPublicCollections.mockReturnValue({ data: undefined })

    const { container } = renderWithProviders(
      <ProfileCollections username="alice" isOwner={false} />
    )
    expect(container.innerHTML).toBe('')
  })
})
