'use client'

import { useState, useMemo, useEffect, useRef } from 'react'
import { Pencil, Check, Loader2 } from 'lucide-react'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { useSuggestEdit } from '../hooks/useSuggestEdit'
import type { EditableEntityType, EditableField, FieldChange } from '../types'
import { EDITABLE_FIELDS } from '../types'

/** Extracts a field value from an entity object, handling nested social fields. */
function getEntityFieldValue(entity: Record<string, unknown>, field: string): string {
  // Direct field
  if (field in entity) {
    return String(entity[field] ?? '')
  }
  // Social fields are nested under entity.social
  const social = entity.social as Record<string, unknown> | undefined
  if (social && field in social) {
    return String(social[field] ?? '')
  }
  return ''
}

interface EntityEditDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  entityType: EditableEntityType
  entityId: number
  entityName: string
  /** The current entity data — used to pre-fill form fields. */
  entity: Record<string, unknown>
  /** Whether the current user can edit directly (trusted_contributor+/admin). */
  canEditDirectly: boolean
  /**
   * Called after a successful edit. Receives `{ applied }` so callers can
   * differentiate a direct (admin/trusted) save — drawer closes silently —
   * from a pending submission, which keeps the in-drawer review banner.
   * Direct saves leave nothing on the page; the parent should render its
   * own page-level success banner via `useEntitySaveSuccessBanner`.
   */
  onSuccess?: (result: { applied: boolean }) => void
  /** When set, the drawer will scroll to and focus this field after opening. */
  focusField?: string
}

export function EntityEditDrawer({
  open,
  onOpenChange,
  entityType,
  entityId,
  entityName,
  entity,
  canEditDirectly,
  onSuccess,
  focusField,
}: EntityEditDrawerProps) {
  const fields = EDITABLE_FIELDS[entityType]
  const suggestEdit = useSuggestEdit()

  // Form state — initialized from entity values
  const [formValues, setFormValues] = useState<Record<string, string>>({})
  const [summary, setSummary] = useState('')
  const [submitted, setSubmitted] = useState(false)

  // Initialize form values when drawer opens
  const initialValues = useMemo(() => {
    const values: Record<string, string> = {}
    for (const field of fields) {
      values[field.key] = getEntityFieldValue(entity, field.key)
    }
    return values
  }, [entity, fields])

  // Track whether we need to focus a field after drawer opens
  const pendingFocusField = useRef<string | undefined>(undefined)

  // Reset form when drawer opens
  const handleOpenChange = (isOpen: boolean) => {
    if (isOpen) {
      setFormValues({})
      setSummary('')
      setSubmitted(false)
      suggestEdit.reset()
      pendingFocusField.current = focusField
    } else {
      pendingFocusField.current = undefined
    }
    onOpenChange(isOpen)
  }

  // Scroll to and focus the target field after the drawer opens and animates in
  useEffect(() => {
    if (!open || !pendingFocusField.current) return

    const fieldKey = pendingFocusField.current
    // Delay to allow the sheet open animation to complete
    const timer = setTimeout(() => {
      const input = document.getElementById(`edit-${fieldKey}`)
      if (input) {
        input.scrollIntoView({ behavior: 'smooth', block: 'center' })
        input.focus()
        pendingFocusField.current = undefined
      }
    }, 300)

    return () => clearTimeout(timer)
  }, [open])

  // Get current value (edited or initial)
  const getValue = (key: string) => formValues[key] ?? initialValues[key] ?? ''

  // Compute changed fields
  const changes: FieldChange[] = useMemo(() => {
    const result: FieldChange[] = []
    for (const field of fields) {
      const currentVal = getValue(field.key)
      const originalVal = initialValues[field.key] ?? ''
      if (currentVal !== originalVal) {
        result.push({
          field: field.key,
          old_value: originalVal || null,
          new_value: currentVal || null,
        })
      }
    }
    return result
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formValues, initialValues, fields])

  const hasChanges = changes.length > 0
  const canSubmit = hasChanges && summary.trim().length > 0 && !suggestEdit.isPending

  const handleSubmit = () => {
    if (!canSubmit) return

    suggestEdit.mutate(
      {
        entityType,
        entityId,
        changes,
        summary: summary.trim(),
      },
      {
        onSuccess: (data) => {
          setSubmitted(true)
          if (data.applied) {
            // Direct edit — close after brief success message. The brief
            // in-drawer flash is intentional: the page-level success banner
            // (rendered by the parent via `useEntitySaveSuccessBanner`) is
            // what carries the confirmation forward after the drawer closes.
            setTimeout(() => {
              onOpenChange(false)
              onSuccess?.({ applied: true })
            }, 1000)
          } else {
            onSuccess?.({ applied: false })
          }
        },
      }
    )
  }

  const groupedFields = useMemo(() => {
    const groups: Record<string, EditableField[]> = {}
    for (const field of fields) {
      const group = field.group ?? 'info'
      if (!groups[group]) groups[group] = []
      groups[group].push(field)
    }
    return groups
  }, [fields])

  const groupLabels: Record<string, string> = {
    info: 'Basic Info',
    details: 'Details',
    social: 'Links & Social',
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-lg overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <Pencil className="h-4 w-4" />
            Edit {entityType.charAt(0).toUpperCase() + entityType.slice(1)}
          </SheetTitle>
          <SheetDescription>
            {canEditDirectly
              ? `Changes will be applied directly to "${entityName}".`
              : `Your edit will be submitted for review.`}
          </SheetDescription>
        </SheetHeader>

        {/* Success state */}
        {submitted && suggestEdit.isSuccess && (
          <div className="mx-4 rounded-md border border-green-800 bg-green-950/50 p-4">
            <div className="flex items-center gap-2 text-green-400">
              <Check className="h-4 w-4" />
              <span className="font-medium">
                {suggestEdit.data?.applied
                  ? 'Changes applied!'
                  : 'Edit submitted for review'}
              </span>
            </div>
            {!suggestEdit.data?.applied && (
              <p className="mt-1 text-sm text-muted-foreground">
                An admin will review your changes. You can track your pending edits in your profile.
              </p>
            )}
          </div>
        )}

        {/* Error state */}
        {suggestEdit.isError && (
          <div className="mx-4 rounded-md border border-red-800 bg-red-950/50 p-4">
            <p className="text-sm text-red-400">
              {(suggestEdit.error as Error)?.message || 'Failed to submit edit'}
            </p>
          </div>
        )}

        {/* Form fields */}
        {!submitted && (
          <div className="flex-1 space-y-6 overflow-y-auto px-4 pb-4">
            {Object.entries(groupedFields).map(([group, groupFields]) => (
              <div key={group} className="space-y-3">
                <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  {groupLabels[group] ?? group}
                </h3>
                {groupFields.map((field) => {
                  const value = getValue(field.key)
                  const original = initialValues[field.key] ?? ''
                  const isChanged = value !== original

                  return (
                    <div key={field.key} className="space-y-1.5">
                      <Label
                        htmlFor={`edit-${field.key}`}
                        className={isChanged ? 'text-blue-400' : ''}
                      >
                        {field.label}
                        {isChanged && <span className="ml-1 text-xs">(changed)</span>}
                      </Label>
                      {field.type === 'textarea' ? (
                        <Textarea
                          id={`edit-${field.key}`}
                          value={value}
                          onChange={(e) =>
                            setFormValues((prev) => ({ ...prev, [field.key]: e.target.value }))
                          }
                          placeholder={field.placeholder}
                          rows={4}
                          className={isChanged ? 'border-blue-800' : ''}
                        />
                      ) : (
                        <Input
                          id={`edit-${field.key}`}
                          type="text"
                          value={value}
                          onChange={(e) =>
                            setFormValues((prev) => ({ ...prev, [field.key]: e.target.value }))
                          }
                          placeholder={field.placeholder}
                          className={isChanged ? 'border-blue-800' : ''}
                        />
                      )}
                    </div>
                  )
                })}
              </div>
            ))}

            {/* Diff preview */}
            {hasChanges && (
              <div className="space-y-2">
                <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Changes Preview
                </h3>
                <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
                  {changes.map((change) => {
                    const fieldDef = fields.find((f) => f.key === change.field)
                    return (
                      <div key={change.field} className="mb-2 last:mb-0">
                        <span className="font-medium">{fieldDef?.label ?? change.field}:</span>
                        <div className="ml-2">
                          {change.old_value && (
                            <div className="text-red-400 line-through">
                              {String(change.old_value)}
                            </div>
                          )}
                          {change.new_value && (
                            <div className="text-green-400">{String(change.new_value)}</div>
                          )}
                          {!change.old_value && !change.new_value && (
                            <span className="text-muted-foreground italic">cleared</span>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Edit summary */}
            <div className="space-y-1.5">
              <Label htmlFor="edit-summary" className="text-foreground">
                Why are you making this change? <span className="text-red-400">*</span>
              </Label>
              <Textarea
                id="edit-summary"
                value={summary}
                onChange={(e) => setSummary(e.target.value)}
                placeholder="e.g., Fix misspelled name, add missing social link..."
                rows={2}
              />
              <p className="text-xs text-muted-foreground">
                Required. Helps reviewers understand your edit.
              </p>
            </div>
          </div>
        )}

        {!submitted && (
          <SheetFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button onClick={handleSubmit} disabled={!canSubmit}>
              {suggestEdit.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {canEditDirectly ? 'Save Changes' : 'Submit for Review'}
            </Button>
          </SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  )
}
