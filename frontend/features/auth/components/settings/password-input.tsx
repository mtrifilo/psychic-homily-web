'use client'

import { useState, type ChangeEvent, type FocusEvent } from 'react'
import { Eye, EyeOff, Lock } from 'lucide-react'
import { Input } from '@/components/ui/input'

interface PasswordInputProps {
  id: string
  name: string
  value: string
  placeholder: string
  invalid: boolean
  onChange: (event: ChangeEvent<HTMLInputElement>) => void
  onBlur: (event: FocusEvent<HTMLInputElement>) => void
}

/**
 * The Settings password forms' input: a lock icon, the field, and a button
 * that shows or hides what was typed. Each input keeps its own visibility.
 */
export function PasswordInput({
  id,
  name,
  value,
  placeholder,
  invalid,
  onChange,
  onBlur,
}: PasswordInputProps) {
  const [visible, setVisible] = useState(false)

  return (
    <div className="relative">
      <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        id={id}
        name={name}
        type={visible ? 'text' : 'password'}
        placeholder={placeholder}
        value={value}
        onBlur={onBlur}
        onChange={onChange}
        className="pl-10 pr-10"
        aria-invalid={invalid}
      />
      <button
        type="button"
        onClick={() => setVisible(!visible)}
        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
        aria-label={visible ? 'Hide password' : 'Show password'}
        aria-pressed={visible}
      >
        {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}
