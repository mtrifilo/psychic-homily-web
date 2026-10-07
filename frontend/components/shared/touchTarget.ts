/**
 * Grows a control's tap target to at least 24x24 CSS px (the WCAG 2.5.8
 * minimum target size) on coarse pointers, without changing anything visible.
 *
 * The extra area is a transparent `::after` box centred on the control's
 * padding box: as large as that box, and never smaller than 24px on either
 * axis. A tap on it is a tap on the control. It takes no space in layout and
 * paints nothing, so the glyph, the border, the focus ring and the
 * surrounding layout are identical on every pointer type. Every utility is
 * gated on `pointer-coarse`, so fine pointers get none of it.
 *
 * The host must not already use its `::after` or be positioned (`absolute`,
 * `fixed`, `sticky`), because `relative` replaces that positioning on coarse
 * pointers. Its neighbours must sit far enough away that the grown box does
 * not cover them: the box reaches `(24 - padding-box size) / 2` px past each
 * undersized edge.
 */
export const COARSE_POINTER_HIT_AREA_CLASS =
  'pointer-coarse:relative pointer-coarse:after:absolute pointer-coarse:after:top-1/2 pointer-coarse:after:left-1/2 pointer-coarse:after:size-full pointer-coarse:after:min-h-6 pointer-coarse:after:min-w-6 pointer-coarse:after:-translate-1/2'
