import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SelectGroup,
  SelectLabel,
} from './select'

// Radix Select drives its popover with APIs jsdom doesn't implement
// (scrollIntoView, pointer capture). Those stubs live in test/setup.ts so
// every Radix-popover component test shares them.

function renderSelect(props: React.ComponentProps<typeof Select> = {}) {
  return render(
    <Select {...props}>
      <SelectTrigger className="custom-class">
        <SelectValue placeholder="Choose one" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>Broadcast type</SelectLabel>
          <SelectItem value="terrestrial">Terrestrial</SelectItem>
          <SelectItem value="internet">Internet</SelectItem>
          <SelectItem value="both">Both</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

describe('Select', () => {
  it('renders the trigger with placeholder text', () => {
    renderSelect()
    expect(screen.getByRole('combobox')).toHaveTextContent('Choose one')
  })

  it('merges a custom className on the trigger', () => {
    renderSelect()
    expect(screen.getByRole('combobox')).toHaveClass('custom-class')
  })

  it('reflects the selected value in the trigger', () => {
    renderSelect({ defaultValue: 'internet' })
    expect(screen.getByRole('combobox')).toHaveTextContent('Internet')
  })

  it('honors the disabled prop', () => {
    renderSelect({ disabled: true })
    expect(screen.getByRole('combobox')).toBeDisabled()
  })

  it('renders portal items and the group label when open', () => {
    renderSelect({ open: true, defaultValue: 'internet' })
    expect(screen.getByText('Broadcast type')).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Terrestrial' })
    ).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Both' })).toBeInTheDocument()
  })

  it('merges a custom className on the content', () => {
    const { baseElement } = render(
      <Select open>
        <SelectTrigger>
          <SelectValue placeholder="Choose one" />
        </SelectTrigger>
        <SelectContent className="custom-content">
          <SelectItem value="internet">Internet</SelectItem>
        </SelectContent>
      </Select>
    )
    expect(
      baseElement.querySelector('[data-slot="select-content"]')
    ).toHaveClass('custom-content')
  })
})
