'use client'

import {
  REPLY_PERMISSION_LABELS,
  REPLY_PERMISSION_VALUES,
  type ReplyPermission,
} from '../types'

interface ReplyPermissionSelectProps {
  value: ReplyPermission
  onChange: (value: ReplyPermission) => void
  id?: string
  disabled?: boolean
  className?: string
  ariaLabel?: string
}

/**
 * Dropdown for selecting who can reply to a comment. PSY-296.
 *
 * Uses a native <select> to avoid dragging in a full Select primitive for
 * what is a simple 3-option control.
 */
export function ReplyPermissionSelect({
  value,
  onChange,
  id,
  disabled = false,
  className,
  ariaLabel,
}: ReplyPermissionSelectProps) {
  return (
    <select
      id={id}
      value={value}
      onChange={(e) => onChange(e.target.value as ReplyPermission)}
      disabled={disabled}
      aria-label={ariaLabel ?? 'Who can reply'}
      data-testid="reply-permission-select"
      className={
        className ??
        'h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50'
      }
    >
      {REPLY_PERMISSION_VALUES.map((opt) => (
        <option key={opt} value={opt}>
          {REPLY_PERMISSION_LABELS[opt]}
        </option>
      ))}
    </select>
  )
}
