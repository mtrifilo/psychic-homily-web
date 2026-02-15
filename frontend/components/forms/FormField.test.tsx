import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { useForm } from '@tanstack/react-form'
import { FieldInfo, FormField } from './FormField'

// Minimal mock field for FieldInfo tests
function makeMockField(overrides: {
  errors?: unknown[]
  isTouched?: boolean
  isValidating?: boolean
}) {
  return {
    state: {
      meta: {
        errors: overrides.errors ?? [],
        isTouched: overrides.isTouched ?? false,
        isValidating: overrides.isValidating ?? false,
      },
      value: '',
    },
    name: 'test-field',
    handleBlur: vi.fn(),
    handleChange: vi.fn(),
  } as any // eslint-disable-line @typescript-eslint/no-explicit-any
}

describe('FieldInfo', () => {
  it('returns null when no errors and not validating', () => {
    const field = makeMockField({})
    const { container } = renderWithProviders(<FieldInfo field={field} />)
    expect(container.innerHTML).toBe('')
  })

  it('shows errors when field is touched and has errors', () => {
    const field = makeMockField({ isTouched: true, errors: ['Required field'] })
    renderWithProviders(<FieldInfo field={field} />)
    expect(screen.getByRole('alert')).toHaveTextContent('Required field')
  })

  it('does not show errors when field is not touched', () => {
    const field = makeMockField({ isTouched: false, errors: ['Required field'] })
    const { container } = renderWithProviders(<FieldInfo field={field} />)
    expect(container.querySelector('[role="alert"]')).toBeNull()
  })

  it('deduplicates identical error messages', () => {
    const field = makeMockField({
      isTouched: true,
      errors: ['Email is required', 'Email is required'],
    })
    renderWithProviders(<FieldInfo field={field} />)
    expect(screen.getByRole('alert')).toHaveTextContent('Email is required')
    // Should NOT contain comma-separated duplicate
    expect(screen.getByRole('alert').textContent).toBe('Email is required')
  })

  it('deduplicates mixed string and object errors with same message', () => {
    const field = makeMockField({
      isTouched: true,
      errors: ['Invalid', { message: 'Invalid' }],
    })
    renderWithProviders(<FieldInfo field={field} />)
    expect(screen.getByRole('alert').textContent).toBe('Invalid')
  })

  it('joins multiple different errors with commas', () => {
    const field = makeMockField({
      isTouched: true,
      errors: ['Too short', 'Must contain a number'],
    })
    renderWithProviders(<FieldInfo field={field} />)
    expect(screen.getByRole('alert')).toHaveTextContent('Too short, Must contain a number')
  })

  it('shows "Validating..." when field is validating', () => {
    const field = makeMockField({ isValidating: true })
    renderWithProviders(<FieldInfo field={field} />)
    expect(screen.getByText('Validating...')).toBeInTheDocument()
  })
})

// Integration test with real TanStack Form field via FormField component
function TestFormField({ type, onEnterPress, disabled }: {
  type?: 'text' | 'textarea' | 'date' | 'time'
  onEnterPress?: () => void
  disabled?: boolean
}) {
  const form = useForm({
    defaultValues: { testField: '' },
  })

  return (
    <form.Field name="testField">
      {field => (
        <FormField
          field={field}
          label="Test Label"
          type={type}
          placeholder="Enter value"
          onEnterPress={onEnterPress}
          disabled={disabled}
        />
      )}
    </form.Field>
  )
}

describe('FormField', () => {
  it('renders label and text input by default', () => {
    renderWithProviders(<TestFormField />)
    expect(screen.getByLabelText('Test Label')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Enter value')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Enter value').tagName).toBe('INPUT')
  })

  it('renders textarea when type is textarea', () => {
    renderWithProviders(<TestFormField type="textarea" />)
    expect(screen.getByPlaceholderText('Enter value').tagName).toBe('TEXTAREA')
  })

  it('renders disabled input when disabled prop is true', () => {
    renderWithProviders(<TestFormField disabled />)
    expect(screen.getByPlaceholderText('Enter value')).toBeDisabled()
  })

  it('calls onEnterPress when Enter key is pressed', async () => {
    const onEnterPress = vi.fn()
    const user = userEvent.setup()
    renderWithProviders(<TestFormField onEnterPress={onEnterPress} />)

    const input = screen.getByPlaceholderText('Enter value')
    await user.click(input)
    await user.keyboard('{Enter}')

    expect(onEnterPress).toHaveBeenCalledOnce()
  })
})
