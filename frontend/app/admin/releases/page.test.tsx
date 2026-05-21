import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminReleasesPage from './page'

// Thin wrapper over ReleaseManagement. Smoke test: the page renders its child
// component without throwing.

vi.mock('@/features/releases/admin/ReleaseManagement', () => ({
  ReleaseManagement: () => <div data-testid="release-management" />,
}))

describe('AdminReleasesPage (app/admin/releases)', () => {
  it('renders ReleaseManagement', () => {
    render(<AdminReleasesPage />)

    expect(screen.getByTestId('release-management')).toBeInTheDocument()
  })
})
