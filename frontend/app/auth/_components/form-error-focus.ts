/**
 * Where focus and scrolling go when an /auth form refuses a submit.
 *
 * The document's `scroll-padding-bottom` reserves the fixed bottom bars (see
 * the geometry contract in app/globals.css), so the scrolls here keep their
 * targets above the tab bar and the cookie banner.
 */

/** The alert elements revealFormAlert has already brought into view. */
const revealedAlerts = new WeakSet<HTMLElement>()

/** What a form exposes about one field's validation result. */
interface FieldMetaReader<Name extends string> {
  getFieldMeta(name: Name): { errors: readonly unknown[] } | undefined
}

/**
 * Focuses the first field, in the order the form renders them, whose
 * validation result holds an error, and centres it in the visible area so the
 * error line rendered under it shows too.
 *
 * Read from the form's own field state rather than from `aria-invalid` in the
 * DOM, because the form reports an invalid submit before React has rendered
 * the errors it found.
 *
 * `fieldIds` pairs each field name with the id of the control that takes its
 * focus, in render order. A submit with no field errors moves nothing.
 */
export function focusFirstInvalidField<Name extends string>(
  form: FieldMetaReader<Name>,
  fieldIds: readonly (readonly [name: Name, id: string])[]
): void {
  const invalid = fieldIds.find(
    ([name]) => (form.getFieldMeta(name)?.errors.length ?? 0) > 0
  )
  const control = invalid && document.getElementById(invalid[1])
  if (!control) return
  control.focus({ preventScroll: true })
  control.scrollIntoView({ block: 'center' })
}

/**
 * A ref callback for a form's error alert. As the alert appears it is
 * scrolled into view, and then the focused control inside the same form is
 * scrolled back into view if that moved it out, so the focused control always
 * shows and the alert shows wherever both fit.
 *
 * Focus is left where it is: the alert is `role="alert"`, which announces it
 * once, and moving focus onto it would announce it a second time.
 *
 * A module-level function, so its identity is stable and React calls it only
 * when the alert mounts and unmounts, never on a re-render. React also calls
 * it again with the same element when a hidden page is shown again, which is
 * not a new message, so each element is revealed once.
 */
export function revealFormAlert(alert: HTMLElement | null): void {
  if (!alert || revealedAlerts.has(alert)) return
  revealedAlerts.add(alert)
  const focused = document.activeElement
  alert.scrollIntoView({ block: 'nearest' })
  if (focused instanceof HTMLElement && alert.closest('form')?.contains(focused)) {
    focused.scrollIntoView({ block: 'nearest' })
  }
}
