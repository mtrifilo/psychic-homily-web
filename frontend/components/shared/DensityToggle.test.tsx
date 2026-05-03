import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DensityToggle } from './DensityToggle'

const mockOnDensityChange = vi.fn()

describe('DensityToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders all three density options', () => {
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)
    expect(screen.getByText('Compact')).toBeInTheDocument()
    expect(screen.getByText('Comfortable')).toBeInTheDocument()
    expect(screen.getByText('Expanded')).toBeInTheDocument()
  })

  it('has radiogroup role with aria-label', () => {
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)
    const group = screen.getByRole('radiogroup')
    expect(group).toHaveAttribute('aria-label', 'Display density')
  })

  it('renders three radio buttons', () => {
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)
    const radios = screen.getAllByRole('radio')
    expect(radios).toHaveLength(3)
  })

  it('marks the current density as checked', () => {
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)
    const comfortable = screen.getByRole('radio', { name: 'Comfortable' })
    const compact = screen.getByRole('radio', { name: 'Compact' })
    const expanded = screen.getByRole('radio', { name: 'Expanded' })

    expect(comfortable).toHaveAttribute('aria-checked', 'true')
    expect(compact).toHaveAttribute('aria-checked', 'false')
    expect(expanded).toHaveAttribute('aria-checked', 'false')
  })

  it('marks compact as checked when density is compact', () => {
    render(<DensityToggle density="compact" onDensityChange={mockOnDensityChange} />)
    expect(screen.getByRole('radio', { name: 'Compact' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'Comfortable' })).toHaveAttribute('aria-checked', 'false')
  })

  it('calls onDensityChange with compact when Compact is clicked', async () => {
    const user = userEvent.setup()
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)

    await user.click(screen.getByText('Compact'))
    expect(mockOnDensityChange).toHaveBeenCalledWith('compact')
  })

  it('calls onDensityChange with expanded when Expanded is clicked', async () => {
    const user = userEvent.setup()
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)

    await user.click(screen.getByText('Expanded'))
    expect(mockOnDensityChange).toHaveBeenCalledWith('expanded')
  })

  it('applies custom className', () => {
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} className="mt-4" />)
    const group = screen.getByRole('radiogroup')
    expect(group.className).toContain('mt-4')
  })

  it('all buttons have type="button"', () => {
    render(<DensityToggle density="comfortable" onDensityChange={mockOnDensityChange} />)
    const radios = screen.getAllByRole('radio')
    for (const radio of radios) {
      expect(radio).toHaveAttribute('type', 'button')
    }
  })

  // PSY-556: when the surrounding view doesn't apply density (e.g. a list
  // layout), the toggle stays mounted but disabled so the toolbar doesn't
  // shift between modes. Persisted selection is preserved by the parent.
  describe('when disabled', () => {
    it('keeps all radios in the DOM (no conditional unmount)', () => {
      render(
        <DensityToggle
          density="comfortable"
          onDensityChange={mockOnDensityChange}
          disabled
          disabledTooltip="Density only applies to grid view"
        />
      )
      expect(screen.getAllByRole('radio')).toHaveLength(3)
    })

    it('marks the radiogroup aria-disabled and disables each button', () => {
      render(
        <DensityToggle
          density="comfortable"
          onDensityChange={mockOnDensityChange}
          disabled
        />
      )
      expect(screen.getByRole('radiogroup')).toHaveAttribute('aria-disabled', 'true')
      for (const radio of screen.getAllByRole('radio')) {
        expect(radio).toBeDisabled()
      }
    })

    it('does not call onDensityChange when a disabled radio is clicked', async () => {
      const user = userEvent.setup()
      render(
        <DensityToggle
          density="comfortable"
          onDensityChange={mockOnDensityChange}
          disabled
        />
      )
      await user.click(screen.getByRole('radio', { name: 'Compact' }))
      expect(mockOnDensityChange).not.toHaveBeenCalled()
    })

    it('preserves the current selection (aria-checked still reflects density)', () => {
      render(
        <DensityToggle
          density="expanded"
          onDensityChange={mockOnDensityChange}
          disabled
        />
      )
      expect(screen.getByRole('radio', { name: 'Expanded' })).toHaveAttribute(
        'aria-checked',
        'true'
      )
    })
  })
})
