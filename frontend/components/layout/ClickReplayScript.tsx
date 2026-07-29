import { CLICK_REPLAY_SCRIPT } from '@/lib/hydration/clickReplay'

/**
 * Installs the pre-hydration click capture listener.
 *
 * Must render in `<head>`, ahead of every other script: its whole job is to be
 * listening during the window before React wakes up. A plain inline `<script>`
 * — not `next/script` — because even `beforeInteractive` is injected by the
 * framework runtime, which is later than "before anything else". The CSP in
 * `next.config.ts` already allows `'unsafe-inline'` for scripts.
 *
 * See `lib/hydration/clickReplay.ts` for the mechanism and its guarantees.
 */
export function ClickReplayScript() {
  return (
    <script
      id="ph-click-replay"
      // The payload is a module-level constant with no interpolated input.
      dangerouslySetInnerHTML={{ __html: CLICK_REPLAY_SCRIPT }}
    />
  )
}
