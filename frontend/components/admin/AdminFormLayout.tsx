'use client'

import * as React from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Label } from '@/components/ui/label'
import { InlineErrorBanner } from '@/components/shared'
import { cn } from '@/lib/utils'

/**
 * AdminFormLayout — the canonical scaffold for admin entity create/edit forms
 * (PSY-911). Per the PSY-912 Hybrid decision, long multi-field forms use a
 * right-anchored Sheet (`variant="sheet"`, the default) and short confirm /
 * simple-edit forms use a centered Dialog (`variant="modal"`). Either way the
 * chrome is identical: header (title + optional description), a scrollable form
 * body, and a footer button row pinned to the bottom. The caller supplies the
 * fields (composed with AdminFormRow / AdminFormField) and the footer buttons.
 */
export interface AdminFormLayoutProps {
  /** Container shape. Sheet (default) for long forms, Modal for short ones. */
  variant?: 'sheet' | 'modal'
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  /** Top-of-form error banner (e.g. a failed submit). */
  error?: string
  onSubmit: React.FormEventHandler<HTMLFormElement>
  /** The footer button row (e.g. Cancel + Submit). Right-aligned on desktop. */
  footer: React.ReactNode
  children: React.ReactNode
  /** Override the container width/size (e.g. "sm:max-w-xl"). */
  contentClassName?: string
}

export function AdminFormLayout({
  variant = 'sheet',
  open,
  onOpenChange,
  title,
  description,
  error,
  onSubmit,
  footer,
  children,
  contentClassName,
}: AdminFormLayoutProps) {
  // The form body is container-agnostic: a <form> that fills the remaining
  // height (min-h-0 + flex-1 so the body can scroll) with the field content
  // scrolling and the footer pinned to the bottom.
  const body = (
    <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        {error ? <InlineErrorBanner>{error}</InlineErrorBanner> : null}
        {children}
      </div>
      <div className="flex flex-col-reverse gap-2 border-t p-4 sm:flex-row sm:justify-end">
        {footer}
      </div>
    </form>
  )

  if (variant === 'modal') {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          className={cn(
            'flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0',
            contentClassName
          )}
        >
          <DialogHeader className="space-y-1.5 border-b p-4 text-left">
            <DialogTitle className="pr-10">{title}</DialogTitle>
            {description ? (
              <DialogDescription>{description}</DialogDescription>
            ) : (
              // Always provide an accessible description so Radix doesn't warn
              // and screen readers get context even when no visible copy is set.
              <DialogDescription className="sr-only">{title}</DialogDescription>
            )}
          </DialogHeader>
          {body}
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className={cn('flex w-full flex-col gap-0 p-0 sm:max-w-2xl', contentClassName)}
      >
        <SheetHeader className="border-b">
          <SheetTitle className="pr-10">{title}</SheetTitle>
          {description ? (
            <SheetDescription>{description}</SheetDescription>
          ) : (
            <SheetDescription className="sr-only">{title}</SheetDescription>
          )}
        </SheetHeader>
        {body}
      </SheetContent>
    </Sheet>
  )
}

const ROW_COLS: Record<1 | 2 | 3 | 4, string> = {
  1: '',
  2: 'sm:grid-cols-2',
  3: 'sm:grid-cols-3',
  4: 'sm:grid-cols-4',
}

/**
 * AdminFormRow — a responsive field grid. One column on mobile; `cols` columns
 * from the `sm` breakpoint up. Group related fields (e.g. City / State / Country
 * / Timezone) into a single row.
 */
export function AdminFormRow({
  cols = 1,
  className,
  children,
}: {
  cols?: 1 | 2 | 3 | 4
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={cn('grid grid-cols-1 gap-4', ROW_COLS[cols], className)}>
      {children}
    </div>
  )
}

/**
 * AdminFormField — a single labelled control. Pass the control (Input / Select /
 * Textarea / …) as children with an `id` matching `htmlFor`. Include any
 * required marker in the `label` text.
 *
 * Per-field inline errors are intentionally omitted for now: the admin forms
 * use a single form-level error banner (AdminFormLayout's `error`). When a form
 * needs field-level validation display, add an `error` prop here that also wires
 * `aria-invalid` / `aria-describedby` onto the control (don't render an
 * unassociated <p>).
 */
export function AdminFormField({
  label,
  htmlFor,
  className,
  children,
}: {
  label: React.ReactNode
  htmlFor?: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={cn('space-y-2', className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  )
}
