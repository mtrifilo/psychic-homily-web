import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminFestivalsPage from './page'

// Thin wrapper over FestivalManagement. Smoke test: the page renders its child
// component without throwing. The page imports the component through the
// feature barrel (@/features/festivals/admin), so mock that specifier.

vi.mock('@/features/festivals/admin', () => ({
  FestivalManagement: () => <div data-testid="festival-management" />,
}))

describe('AdminFestivalsPage (app/admin/festivals)', () => {
  it('renders FestivalManagement', () => {
    render(<AdminFestivalsPage />)

    expect(screen.getByTestId('festival-management')).toBeInTheDocument()
  })
})
