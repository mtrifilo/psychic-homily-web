import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DensityToggle } from './DensityToggle'

const mockSetDensity = vi.fn()
const mockUseDensity = vi.fn(() => ({
  density: 'comfortable' as const,
  setDensity: mockSetDensity,
}))

vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: (...args: unknown[]) => mockUseDensity(...args),
}))

describe('DensityToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseDensity.mockReturnValue({
      density: 'comfortable',
      setDensity: mockSetDensity,
    })
  })

  it('renders all three density options', () => {
    render(<DensityToggle />)
    expect(screen.getByText('Compact')).toBeInTheDocument()
    expect(screen.getByText('Comfortable')).toBeInTheDocument()
    expect(screen.getByText('Expanded')).toBeInTheDocument()
  })

  it('has radiogroup role with aria-label', () => {
    render(<DensityToggle />)
    const group = screen.getByRole('radiogroup')
    expect(group).toHaveAttribute('aria-label', 'Display density')
  })

  it('renders three radio buttons', () => {
    render(<DensityToggle />)
    const radios = screen.getAllByRole('radio')
    expect(radios).toHaveLength(3)
  })

  it('marks the current density as checked', () => {
    render(<DensityToggle />)
    const comfortable = screen.getByRole('radio', { name: 'Comfortable' })
    const compact = screen.getByRole('radio', { name: 'Compact' })
    const expanded = screen.getByRole('radio', { name: 'Expanded' })

    expect(comfortable).toHaveAttribute('aria-checked', 'true')
    expect(compact).toHaveAttribute('aria-checked', 'false')
    expect(expanded).toHaveAttribute('aria-checked', 'false')
  })

  it('marks compact as checked when density is compact', () => {
    mockUseDensity.mockReturnValue({
      density: 'compact',
      setDensity: mockSetDensity,
    })
    render(<DensityToggle />)
    expect(screen.getByRole('radio', { name: 'Compact' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'Comfortable' })).toHaveAttribute('aria-checked', 'false')
  })

  it('calls setDensity with compact when Compact is clicked', async () => {
    const user = userEvent.setup()
    render(<DensityToggle />)

    await user.click(screen.getByText('Compact'))
    expect(mockSetDensity).toHaveBeenCalledWith('compact')
  })

  it('calls setDensity with expanded when Expanded is clicked', async () => {
    const user = userEvent.setup()
    render(<DensityToggle />)

    await user.click(screen.getByText('Expanded'))
    expect(mockSetDensity).toHaveBeenCalledWith('expanded')
  })

  it('passes storageKey to useDensity', () => {
    render(<DensityToggle storageKey="shows" />)
    expect(mockUseDensity).toHaveBeenCalledWith('shows')
  })

  it('passes undefined storageKey to useDensity when not provided', () => {
    render(<DensityToggle />)
    expect(mockUseDensity).toHaveBeenCalledWith(undefined)
  })

  it('applies custom className', () => {
    render(<DensityToggle className="mt-4" />)
    const group = screen.getByRole('radiogroup')
    expect(group.className).toContain('mt-4')
  })

  it('all buttons have type="button"', () => {
    render(<DensityToggle />)
    const radios = screen.getAllByRole('radio')
    for (const radio of radios) {
      expect(radio).toHaveAttribute('type', 'button')
    }
  })
})
