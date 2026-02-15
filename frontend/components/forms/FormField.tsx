'use client'

import { type AnyFieldApi } from '@tanstack/react-form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getUniqueErrors } from '@/lib/utils/formErrors'

interface FormFieldProps {
  field: AnyFieldApi
  label: string
  type?: 'text' | 'date' | 'time' | 'textarea' | 'number'
  placeholder?: string
  onEnterPress?: () => void
  onChange?: (value: string) => void
  disabled?: boolean
}

/**
 * Display field validation errors
 */
export function FieldInfo({ field }: Readonly<{ field: AnyFieldApi }>) {
  const errors = field.state.meta.errors

  if (field.state.meta.isTouched && errors.length > 0) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {getUniqueErrors(errors)}
      </p>
    )
  }

  if (field.state.meta.isValidating) {
    return <p className="text-sm text-muted-foreground">Validating...</p>
  }

  return null
}

/**
 * Reusable form field component with label, input, and error display
 */
export function FormField({
  field,
  label,
  type = 'text',
  placeholder,
  onEnterPress,
  onChange,
  disabled,
}: Readonly<FormFieldProps>) {
  const handleChange = (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    const value = e.target.value
    field.handleChange(value)
    onChange?.(value)
  }

  const handleKeyDown = (
    e: React.KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => {
    if (e.key === 'Enter' && onEnterPress) {
      e.preventDefault()
      onEnterPress()
    }
  }

  return (
    <div className="space-y-2">
      <Label htmlFor={field.name}>{label}</Label>
      {type === 'textarea' ? (
        <textarea
          className="flex min-h-[100px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:border-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm"
          id={field.name}
          name={field.name}
          value={field.state.value}
          onBlur={field.handleBlur}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          aria-invalid={field.state.meta.errors.length > 0}
          disabled={disabled}
        />
      ) : (
        <Input
          type={type}
          id={field.name}
          name={field.name}
          value={field.state.value}
          onBlur={field.handleBlur}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          aria-invalid={field.state.meta.errors.length > 0}
          disabled={disabled}
        />
      )}
      <FieldInfo field={field} />
    </div>
  )
}

