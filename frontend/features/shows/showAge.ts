/**
 * Which age rule GOVERNS a show: its own door policy, or the room's.
 *
 * The show's `age_requirement` is the per-event OVERRIDE and the venue's
 * `age_policy` is the house default (PSY-1682), so the override wins wherever
 * it is present and the house default is what a show without one falls back
 * to. Whitespace-only values are absences: both columns are contributor-writable
 * free text, and a cell holding a space would otherwise render an empty age.
 *
 * ONE rule, shared by the venue facts line and the discovery rails, because the
 * two sit a screen apart on the show page and a reader who saw `all ages` in
 * one and `21+` in the other would have no way to tell which was the page's
 * answer. The two surfaces still SPELL it differently — the facts line has room
 * to name a disagreement between the halves, a ledger cell does not — and that
 * is a register difference on top of this one derivation, not a second rule.
 */
export function governingAgeRequirement(
  override: string | null | undefined,
  houseDefault: string | null | undefined
): string | null {
  return override?.trim() || houseDefault?.trim() || null
}
