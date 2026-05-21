import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminTagsPage from './page'

// Thin wrapper over TagManagement. Smoke test: the page renders its child
// component without throwing.

vi.mock('@/features/tags/admin/TagManagement', () => ({
  TagManagement: () => <div data-testid="tag-management" />,
}))

describe('AdminTagsPage (app/admin/tags)', () => {
  it('renders TagManagement', () => {
    render(<AdminTagsPage />)

    expect(screen.getByTestId('tag-management')).toBeInTheDocument()
  })
})
