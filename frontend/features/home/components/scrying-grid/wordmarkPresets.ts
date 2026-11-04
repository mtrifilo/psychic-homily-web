/**
 * The Scrying Grid wordmark's fixed-size layouts and one-line text.
 *
 * ISOMORPHIC ON PURPOSE: no `'use client'` here. Server components (the
 * signed-in home's masthead) pass these values to the client wordmark; a value
 * imported from a `'use client'` module arrives in a server module as a client
 * reference, not the value.
 */

/** The wordmark on one line, as the bar mark and the nameplate set it. */
export const WORDMARK_ONE_LINE: readonly string[] = ['PSYCHIC HOMILY']

/** A fixed-size layout of the field, tuned on /admin/hero-lab against the
 *  Product Designs components (bar mark 2240:3336, nameplate 2240:3359). */
export interface WordmarkPreset {
  /** Sampling pitch in CSS px. */
  gapFactor: number
  /** Dark-mode bloom pass. */
  bloom: boolean
  /** The box the preset is tuned for; the container takes it. A caller's
   *  class may cap it (`max-w-*`); the ink re-fits a smaller box. */
  boxClassName: string
  /** Text size of the fallback (no JS, or the canvas could not sample). */
  fallbackClassName: string
}

/**
 * - `bar`: one line in a 306x30 box, a 28px cap. Pitch 3px (about 9 to 10
 *   cells per letter height). No bloom: at a 3px pitch the halo fills the
 *   gaps between cells and blurs the dot texture, adding no visible glow.
 * - `nameplate`: one line in a 1084x100 box, a 100px cap. Pitch 4px. Bloom
 *   in dark mode.
 *
 * Measured on /admin/hero-lab at dpr 2: the bar's cells span about 298x26
 * and the nameplate's about 1062x95, each centred, at exactly the pitch.
 */
export const WORDMARK_PRESETS = {
  bar: {
    gapFactor: 3,
    bloom: false,
    boxClassName: 'h-[30px] w-[306px]',
    fallbackClassName: 'text-[22px]',
  },
  nameplate: {
    gapFactor: 4,
    bloom: true,
    boxClassName: 'h-[100px] w-[1084px]',
    fallbackClassName: 'text-[88px]',
  },
} as const satisfies Record<string, WordmarkPreset>

export type WordmarkPresetName = keyof typeof WORDMARK_PRESETS
