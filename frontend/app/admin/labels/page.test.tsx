import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminLabelsPage from './page'

// Thin wrapper over LabelManagement. Smoke test: the page renders its child
// component without throwing.

vi.mock('@/features/labels/admin/LabelManagement', () => ({
  LabelManagement: () => <div data-testid="label-management" />,
}))

describe('AdminLabelsPage (app/admin/labels)', () => {
  it('renders LabelManagement', () => {
    render(<AdminLabelsPage />)

    expect(screen.getByTestId('label-management')).toBeInTheDocument()
  })
})
