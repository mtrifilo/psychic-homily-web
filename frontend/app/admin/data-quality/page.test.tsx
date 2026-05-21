import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import DataQualityPage from './page'

// Thin wrapper over DataQualityDashboard. Smoke test: the page renders its
// child component without throwing.

vi.mock('@/app/admin/data-quality/_components/DataQualityDashboard', () => ({
  DataQualityDashboard: () => <div data-testid="data-quality-dashboard" />,
}))

describe('DataQualityPage (app/admin/data-quality)', () => {
  it('renders DataQualityDashboard', () => {
    render(<DataQualityPage />)

    expect(screen.getByTestId('data-quality-dashboard')).toBeInTheDocument()
  })
})
