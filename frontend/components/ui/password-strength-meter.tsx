'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'
import { Check, X } from 'lucide-react'

interface PasswordStrengthMeterProps {
  password: string
  className?: string
  showRequirements?: boolean
}

interface PasswordRequirement {
  label: string
  met: boolean
}

// Calculate password strength score (0-100)
function calculateStrength(password: string): number {
  if (!password) return 0

  let score = 0
  const length = password.length

  // Length scoring (up to 40 points)
  if (length >= 12) score += 20
  if (length >= 16) score += 10
  if (length >= 20) score += 10

  // Character variety scoring (up to 40 points)
  const hasLower = /[a-z]/.test(password)
  const hasUpper = /[A-Z]/.test(password)
  const hasDigit = /[0-9]/.test(password)
  const hasSpecial = /[^a-zA-Z0-9]/.test(password)

  if (hasLower) score += 10
  if (hasUpper) score += 10
  if (hasDigit) score += 10
  if (hasSpecial) score += 10

  // Entropy bonus (up to 20 points)
  const uniqueChars = new Set(password).size
  const entropyRatio = uniqueChars / length

  if (entropyRatio > 0.5 && length >= 12) score += 10
  if (entropyRatio > 0.7 && length >= 16) score += 10

  return Math.min(score, 100)
}

// Get strength label and color based on score
function getStrengthInfo(score: number): { label: string; color: string; bgColor: string } {
  if (score < 30) {
    return { label: 'Weak', color: 'text-red-500', bgColor: 'bg-red-500' }
  }
  if (score < 50) {
    return { label: 'Fair', color: 'text-orange-500', bgColor: 'bg-orange-500' }
  }
  if (score < 70) {
    return { label: 'Good', color: 'text-yellow-500', bgColor: 'bg-yellow-500' }
  }
  if (score < 90) {
    return { label: 'Strong', color: 'text-green-500', bgColor: 'bg-green-500' }
  }
  return { label: 'Excellent', color: 'text-emerald-500', bgColor: 'bg-emerald-500' }
}

// Check password requirements
function checkRequirements(password: string): PasswordRequirement[] {
  return [
    {
      label: 'At least 12 characters',
      met: password.length >= 12,
    },
    {
      label: 'Contains uppercase and lowercase',
      met: /[a-z]/.test(password) && /[A-Z]/.test(password),
    },
    {
      label: 'Contains a number',
      met: /[0-9]/.test(password),
    },
    {
      label: 'Contains a special character',
      met: /[^a-zA-Z0-9]/.test(password),
    },
  ]
}

export function PasswordStrengthMeter({
  password,
  className,
  showRequirements = true,
}: PasswordStrengthMeterProps) {
  const strength = calculateStrength(password)
  const { label, color, bgColor } = getStrengthInfo(strength)
  const requirements = checkRequirements(password)

  // Don't show anything if password is empty
  if (!password) return null

  return (
    <div className={cn('space-y-2', className)}>
      {/* Strength bar */}
      <div className="space-y-1">
        <div className="flex justify-between text-xs">
          <span className="text-muted-foreground">Password strength</span>
          <span className={cn('font-medium', color)}>{label}</span>
        </div>
        <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
          <div
            className={cn('h-full rounded-full transition-all duration-300', bgColor)}
            style={{ width: `${strength}%` }}
          />
        </div>
      </div>

      {/* Requirements checklist */}
      {showRequirements && (
        <ul className="space-y-1 text-xs">
          {requirements.map(req => (
            <li
              key={req.label}
              className={cn(
                'flex items-center gap-1.5 transition-colors',
                req.met ? 'text-green-600 dark:text-green-500' : 'text-muted-foreground'
              )}
            >
              {req.met ? (
                <Check className="h-3 w-3" />
              ) : (
                <X className="h-3 w-3" />
              )}
              {req.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// Export utility functions for use in validation
export { calculateStrength, checkRequirements, getStrengthInfo }
