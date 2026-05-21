import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import AdminRadioPage from './page'

// Thin wrapper over RadioManagement. Smoke test: the page renders its child
// component without throwing.

vi.mock('./_components/RadioManagement', () => ({
  RadioManagement: () => <div data-testid="radio-management" />,
}))

describe('AdminRadioPage (app/admin/radio)', () => {
  it('renders RadioManagement', () => {
    render(<AdminRadioPage />)

    expect(screen.getByTestId('radio-management')).toBeInTheDocument()
  })
})
