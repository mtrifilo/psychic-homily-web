import { useState } from 'react'
import { Input } from '../ui/input'
import { Label } from '../ui/label'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Eye, EyeOff, Check, X } from 'lucide-react'
import { validateToken } from '../../lib/schemas/settings'
import { cn } from '../../lib/utils'

interface TokenInputProps {
  id: string
  label: string
  description?: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

export function TokenInput({
  id,
  label,
  description,
  value,
  onChange,
  placeholder = 'phk_...',
}: TokenInputProps) {
  const [showToken, setShowToken] = useState(false)
  const [touched, setTouched] = useState(false)

  const validation = validateToken(value)
  const isConfigured = Boolean(value)
  const showError = touched && !validation.valid

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label htmlFor={id} className="text-xs text-muted-foreground">
          {label}
        </Label>
        {isConfigured && validation.valid && (
          <Badge variant="secondary" className="bg-green-100 text-green-700 hover:bg-green-100 dark:bg-green-950/50 dark:text-green-400 dark:hover:bg-green-950/50">
            <Check className="h-3 w-3 mr-1" />
            Configured
          </Badge>
        )}
      </div>
      {description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
      <div className="relative">
        <Input
          id={id}
          type={showToken ? 'text' : 'password'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onBlur={() => setTouched(true)}
          placeholder={placeholder}
          className={cn(
            'pr-10 font-mono',
            showError && 'border-destructive focus-visible:ring-destructive/50'
          )}
          aria-invalid={showError}
          aria-describedby={showError ? `${id}-error` : undefined}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="absolute right-1 top-1/2 -translate-y-1/2"
          onClick={() => setShowToken(!showToken)}
          aria-label={showToken ? 'Hide token' : 'Show token'}
        >
          {showToken ? (
            <EyeOff className="h-4 w-4 text-muted-foreground" />
          ) : (
            <Eye className="h-4 w-4 text-muted-foreground" />
          )}
        </Button>
      </div>
      {showError && (
        <p id={`${id}-error`} className="text-xs text-destructive flex items-center gap-1">
          <X className="h-3 w-3" />
          {validation.error}
        </p>
      )}
    </div>
  )
}
