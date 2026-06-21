/**
 * Hero Lab — shared constants for the animated-wordmark prototypes.
 *
 * The lab (frontend/app/admin/hero-lab, admin-only) is an experiment gallery
 * that renders five candidate hero treatments for the "PSYCHIC HOMILY" wordmark
 * side by side. Option A (Scrying Grid) ships to the homepage (PSY-1137); B/C/D/E
 * live on here for future design experiments (PSY-1138). Self-contained under a
 * private `_`-prefixed folder. See HeroLab.tsx for the menu and per-effect notes.
 */

/** The wordmark, stacked two lines (sampled / rendered as PSYCHIC over HOMILY). */
export const WORDMARK_LINES = ['PSYCHIC', 'HOMILY'] as const
