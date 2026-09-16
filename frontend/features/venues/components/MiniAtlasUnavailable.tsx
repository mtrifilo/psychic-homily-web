/**
 * What the mini Atlas pane shows when its map cannot be drawn.
 *
 * The map is a second view of the table beside it, so a failure here costs the
 * reader nothing they cannot get from the rows. The message says that rather
 * than apologising, and it replaces the skeleton so a pane that is never going
 * to paint stops pretending it is still loading.
 */
export function MiniAtlasUnavailable() {
  return (
    <div
      role="status"
      data-testid="venue-mini-atlas-unavailable"
      className="absolute inset-0 flex items-center justify-center p-4 text-center text-xs text-muted-foreground"
    >
      <p>The map is unavailable. Every room is in the table.</p>
    </div>
  )
}
