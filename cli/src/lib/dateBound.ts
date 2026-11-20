/**
 * How many months ahead the API accepts a show's `event_date` without the
 * explicit override. Kept equal by hand to `ShowEventDateBoundMonths` in
 * backend/internal/services/catalog/show_date_bound.go, which is the
 * authority; this copy only lets the dry run say what the API will refuse.
 */
export const SHOW_EVENT_DATE_BOUND_MONTHS = 18;

/**
 * The latest `event_date` accepted at `now` without the override: `now` plus
 * the bound in calendar months, in UTC. `setUTCMonth` overflows a short month
 * the same way Go's `AddDate` does (Aug 31 + 18 months is Mar 3), so the two
 * agree on every day.
 */
export function showEventDateBound(now: Date): Date {
  const bound = new Date(now.getTime());
  bound.setUTCMonth(bound.getUTCMonth() + SHOW_EVENT_DATE_BOUND_MONTHS);
  return bound;
}

/**
 * Whether an ISO instant is after the bound. An unreadable value answers
 * false: it is the API's to reject, and the dry run already prints it.
 */
export function isBeyondShowEventDateBound(eventDate: string, now: Date): boolean {
  const at = Date.parse(eventDate);
  return Number.isFinite(at) && at > showEventDateBound(now).getTime();
}
