'use client'

import { useState, useMemo, useEffect, useRef } from 'react'
import { Pencil, Loader2 } from 'lucide-react'
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
import { StatusBanner } from '@/components/shared'
import { useSuggestEdit } from '../hooks/useSuggestEdit'
import { useShowEdit } from '../hooks/useShowEdit'
import type { EditableEntityType, EditableField, FieldChange, EntityEditSuccess } from '../types'
import { EDITABLE_FIELDS, fieldChangeValue, validateFieldValue } from '../types'
import { useDismissTimer } from '@/lib/hooks/common'
import { isConflictError } from '@/lib/api'

// How long the in-drawer success flash shows before the drawer closes itself.
const APPLIED_CLOSE_DELAY_MS = 1000

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
  onSuccess?: (result: EntityEditSuccess) => void
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
  // PSY-563: shows route through the direct-save endpoint, not the
  // suggest-edit pipeline. Both hooks are called unconditionally so the
  // hook order stays stable; only one is dispatched per drawer instance.
  const suggestEdit = useSuggestEdit()
  const showEdit = useShowEdit()
  const editMutation = entityType === 'show' ? showEdit : suggestEdit

  // Form state — initialized from entity values
  const [formValues, setFormValues] = useState<Record<string, string>>({})
  const [summary, setSummary] = useState('')
  const [submitted, setSubmitted] = useState(false)
  // PSY-599: track which validated fields the user has interacted with so we
  // defer the inline error until they've had a chance to finish typing.
  // Mirrors the touched-state pattern in `CollectionDetail` (PSY-371).
  const [touchedFields, setTouchedFields] = useState<Record<string, boolean>>({})

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
      setTouchedFields({})
      // Reset both — only one is "live" but resetting both is safe
      // and keeps state clean if the parent ever switches entityType
      // on this same drawer instance (unlikely but defensible).
      suggestEdit.reset()
      showEdit.reset()
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
      // Compare the values that will actually be SENT, not the raw input
      // strings. For every other field type those are the same comparison,
      // because the conversion is the identity. For a numeric field they are
      // not: "550 " and "550" are different strings and the same number, and
      // "   " and "" are different strings that both mean "cleared". Comparing
      // strings there would file a pending edit that changes nothing and put it
      // in front of an admin as "550 -> 550" or as a spurious "cleared".
      const next = fieldChangeValue(field, currentVal)
      const previous = fieldChangeValue(field, originalVal)
      if (next !== previous) {
        result.push({
          field: field.key,
          old_value: previous,
          new_value: next,
        })
      }
    }
    return result
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formValues, initialValues, fields])

  // PSY-599: validate constrained fields client-side so a malformed value
  // disables Submit and surfaces an inline hint, instead of forcing the user
  // through a server roundtrip that returns a confusing 422 banner. We only
  // consider fields whose RAW input the user touched. Pre-existing bad values
  // on the entity stay tolerated (the backend grandfathers them too).
  //
  // Raw input, not the converted value, is the right trigger here even though
  // `changes` compares converted values: someone who typed "550abc" over "550"
  // has changed nothing that can be sent, but they have earned an error message
  // rather than a silently dead Submit button.
  const fieldErrors = useMemo(() => {
    const errors: Record<string, string> = {}
    for (const field of fields) {
      const currentVal = getValue(field.key)
      const originalVal = initialValues[field.key] ?? ''
      if (currentVal === originalVal) continue
      const err = validateFieldValue(field, currentVal)
      if (err !== null) errors[field.key] = err
    }
    return errors
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [formValues, initialValues, fields])

  // One definition of "this field changed", shared by the label affordance and
  // the payload. Deriving the indicator from the raw strings instead would let
  // the drawer highlight a field blue, show no error, and still refuse to
  // submit, with nothing on screen explaining why (typing a trailing space into
  // a numeric field does exactly that).
  const changedFieldKeys = useMemo(
    () => new Set(changes.map((change) => change.field)),
    [changes]
  )

  // Hold the in-drawer success flash, then close. Timer must not outlive
  // unmount, see useDismissTimer (PSY-1664).
  const { schedule: scheduleAppliedClose } = useDismissTimer(() => {
    onOpenChange(false)
    onSuccess?.({ applied: true })
  }, APPLIED_CLOSE_DELAY_MS)

  const hasChanges = changes.length > 0
  const hasFieldErrors = Object.keys(fieldErrors).length > 0
  const canSubmit =
    hasChanges &&
    !hasFieldErrors &&
    summary.trim().length > 0 &&
    !editMutation.isPending

  const handleSubmit = () => {
    if (!canSubmit) return

    const onMutationSuccess = (data: { applied: boolean }) => {
      setSubmitted(true)
      if (data.applied) {
        // Direct edit — close after brief success message. The brief
        // in-drawer flash is intentional: the page-level success banner
        // (rendered by the parent via `useEntitySaveSuccessBanner`) is
        // what carries the confirmation forward after the drawer closes.
        scheduleAppliedClose()
      } else {
        onSuccess?.({ applied: false })
      }
    }

    if (entityType === 'show') {
      // Show direct-save (PSY-461 / PSY-489 / PSY-563). No `entityType`
      // in the payload — the endpoint is /shows/{id} not the polymorphic
      // suggest-edit route.
      showEdit.mutate(
        {
          entityId,
          changes,
          summary: summary.trim(),
        },
        { onSuccess: onMutationSuccess }
      )
    } else {
      suggestEdit.mutate(
        {
          entityType,
          entityId,
          changes,
          summary: summary.trim(),
        },
        { onSuccess: onMutationSuccess }
      )
    }
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

        {/* PSY-575: green StatusBanner for direct-save (admin / trusted),
            amber for "submitted for review" (non-admin). Drawer-close is the
            implicit dismiss — no `dismissAfterMs` on either branch. */}
        {submitted && editMutation.isSuccess && (
          editMutation.data?.applied ? (
            <StatusBanner variant="success" className="mx-4">
              <span className="font-medium text-success-foreground">Changes applied!</span>
            </StatusBanner>
          ) : (
            <StatusBanner variant="pending" className="mx-4">
              <div>
                <span className="font-medium text-pending-foreground">Edit submitted for review</span>
                <p className="mt-1 text-sm text-muted-foreground">
                  An admin will review your changes. You can track your pending edits in your profile.
                </p>
              </div>
            </StatusBanner>
          )
        )}

        {/* Error state. A conflict is not a failure of the submission but a
            report that the entity is not in the state the form described, so the
            server's message says what moved and the line below states what
            already happened: useSuggestEdit refetches the entity on 409 and this
            form reads its previous values from it. Stating the refresh and not
            inviting a resubmit, because a 409 is also how a duplicate queued
            edit is reported, and resubmitting that one fails again. */}
        {editMutation.isError && (
          <div className="mx-4 rounded-md border border-red-800 bg-red-950/50 p-4">
            <p className="text-sm text-red-400">
              {(editMutation.error as Error)?.message || 'Failed to submit edit'}
            </p>
            {isConflictError(editMutation.error) && (
              <p className="mt-1 text-sm text-red-400">
                This form has been refreshed with the current values.
              </p>
            )}
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
                  const isChanged = changedFieldKeys.has(field.key)
                  // PSY-599: the inline error is visible only after the user
                  // has touched the field. Defers the red until they've
                  // finished typing.
                  const fieldError = fieldErrors[field.key]
                  const isTouched = touchedFields[field.key] === true
                  const showFieldError = isTouched && fieldError !== undefined
                  const errorId = `edit-${field.key}-error`
                  // Only validated types carry the touched/error affordance,
                  // so plain text and textarea keep their current behavior.
                  const isValidated = field.type === 'url' || field.type === 'number'

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
                          // Numeric fields stay type="text" with a numeric
                          // inputMode: type="number" hands back "" for any
                          // value the browser considers invalid, which would
                          // make a typo indistinguishable from the clear
                          // gesture on a field where clearing means NULL.
                          type={field.type === 'url' ? 'url' : 'text'}
                          inputMode={
                            field.type === 'url'
                              ? 'url'
                              : field.type === 'number'
                                ? 'numeric'
                                : undefined
                          }
                          value={value}
                          onChange={(e) => {
                            setFormValues((prev) => ({ ...prev, [field.key]: e.target.value }))
                            if (isValidated) {
                              setTouchedFields((prev) => ({ ...prev, [field.key]: true }))
                            }
                          }}
                          onBlur={() => {
                            if (isValidated) {
                              setTouchedFields((prev) => ({ ...prev, [field.key]: true }))
                            }
                          }}
                          placeholder={field.placeholder}
                          maxLength={field.maxLength}
                          className={
                            showFieldError
                              ? 'border-red-800'
                              : isChanged
                                ? 'border-blue-800'
                                : ''
                          }
                          aria-invalid={showFieldError ? true : undefined}
                          aria-describedby={showFieldError ? errorId : undefined}
                          data-testid={`edit-${field.key}-input`}
                        />
                      )}
                      {showFieldError && (
                        <p
                          id={errorId}
                          className="text-xs text-red-400"
                          role="alert"
                          data-testid={`edit-${field.key}-error`}
                        >
                          {fieldError}
                        </p>
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
                          {/* old_value/new_value are `unknown` (the field type
                              is intentionally generic), so each is narrowed to
                              a ReactNode before rendering. Tested against null
                              rather than for truthiness: a numeric field can
                              legitimately carry a falsy value, and rendering
                              that as "cleared" would misreport the edit. Only
                              null is the clear gesture. */}
                          {change.old_value !== null && change.old_value !== undefined && (
                            <div className="text-red-400 line-through">
                              {String(change.old_value)}
                            </div>
                          )}
                          {change.new_value !== null && change.new_value !== undefined ? (
                            <div className="text-green-400">{String(change.new_value)}</div>
                          ) : (
                            // A null new value IS the clear gesture, whatever the
                            // old value was. Requiring BOTH to be null made this
                            // unreachable, so clearing a field rendered as a bare
                            // strikethrough that reads like an unfinished edit.
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
            {/* The helper text names the real audience. It used to say the note
                "helps reviewers understand your edit", which implied moderators
                were the only readers: it is in fact published in the entity's
                edit history, anonymously readable, attributed, and with no edit
                or delete endpoint behind it. Contributors were writing the value
                they were correcting into a box they thought was private. */}
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
                Required. This is published in the public edit history under your
                name and cannot be changed or removed later, so leave out private
                details like a home address.
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
              {editMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {canEditDirectly ? 'Save Changes' : 'Submit for Review'}
            </Button>
          </SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  )
}
