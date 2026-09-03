/**
 * The curated bill roles the API accepts on `show_artists.set_type`.
 *
 * A client-side mirror of the enum published on the shared show `Artist`
 * schema, in the backend's presentation order (top of bill first). The backend
 * is the enforcement point and answers an out-of-vocabulary value with 422;
 * validating here only buys the operator a message that names the vocabulary
 * before a request goes out.
 */
export const SET_TYPE_VOCABULARY = [
  "headliner",
  "direct_support",
  "opener",
  "special_guest",
  "dj",
  "performer",
] as const;

export type SetType = (typeof SET_TYPE_VOCABULARY)[number];

/**
 * The neutral role: "on the bill, slot unknown".
 *
 * It is what every uncurated row holds, so it states nothing about a slot and
 * must never be written back as though somebody had curated it. Absent and
 * `performer` are the two spellings of the same silence.
 */
export const SET_TYPE_UNCURATED: SetType = "performer";

/** Whether `value` is exactly one of the accepted roles. */
export function isValidSetType(value: string): value is SetType {
  return (SET_TYPE_VOCABULARY as readonly string[]).includes(value);
}

/** The vocabulary as a comma-separated list, for validation messages. */
export const SET_TYPE_VOCABULARY_CSV = SET_TYPE_VOCABULARY.join(", ");

/**
 * Whether a stored value says anything at all about an act's slot.
 *
 * THE definition of silence, so the two questions built on it cannot disagree:
 * what gets sent back, and what gets warned about. Absent, blank and
 * `performer` are its three spellings.
 */
function statesASlot(value: string | null | undefined): value is string {
  if (value == null) return false;
  const trimmed = value.trim();
  return trimmed !== "" && trimmed !== SET_TYPE_UNCURATED;
}

/**
 * The role to send back for an act read off a show response, or `undefined`
 * when the stored value curates nothing.
 *
 * Two values resolve to `undefined` and they mean different things:
 *
 * - Silence, per {@link statesASlot}. Only an ABSENT key says "slot unknown" on
 *   the way back in, so the caller must OMIT the field rather than send
 *   `performer` or `null`.
 * - A value outside the vocabulary is a row the API would refuse. Rewriting
 *   the bill is the only way this CLI can add or drop an act, so the choice is
 *   between dropping that one unreadable value and failing the whole edit;
 *   callers drop it and say so out loud. The column carries no CHECK
 *   constraint, so such a row is reachable.
 *
 * The value is judged EXACTLY as stored, untrimmed, which is the same verdict
 * the API gives a role stated on the way in. Only emptiness is forgiving.
 */
export function curatedSetType(
  value: string | null | undefined,
): SetType | undefined {
  if (!statesASlot(value)) return undefined;
  return isValidSetType(value) ? value : undefined;
}

/**
 * Whether a stored value is one this CLI cannot round-trip: it states a slot,
 * but not one the API would accept back.
 *
 * Derived from {@link curatedSetType} rather than restating its test, so the
 * value that gets dropped is exactly the value that gets warned about.
 */
export function isUnroundtrippableSetType(
  value: string | null | undefined,
): boolean {
  return statesASlot(value) && curatedSetType(value) === undefined;
}
