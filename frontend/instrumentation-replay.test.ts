import { describe, it, expect, vi, afterEach } from 'vitest'
import * as Sentry from '@sentry/nextjs'
import { attachReplay } from './instrumentation-replay'

// attachReplay lives in the lazy chunk (PSY-1091). It must preserve the privacy
// posture (mask all text, block all media) when it wires replay in.
const replayIntegrationFn = vi.fn((options) => ({ name: 'Replay', options }))

vi.mock('@sentry/nextjs', () => ({
  addIntegration: vi.fn(),
  replayIntegration: (options: unknown) => replayIntegrationFn(options),
}))

describe('instrumentation-replay.ts', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('adds a masked, media-blocked replay integration', () => {
    attachReplay()

    expect(replayIntegrationFn).toHaveBeenCalledWith(
      expect.objectContaining({
        maskAllText: true,
        blockAllMedia: true,
      })
    )
    expect(Sentry.addIntegration).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Replay' })
    )
  })

  // Replay counts ANY event without a `type` as an error, `captureMessage`
  // included, so a warning is enough to convert a buffering session into an
  // uploaded and then continuously-recorded one. That is right for our own
  // failures and wrong for an event whose trigger is a string an untrusted
  // contributor stored: without this, planting an affiliate tag chooses which
  // visitors get session-recorded, and spends replay quota doing it.
  describe('beforeErrorSampling', () => {
    const samplingFor = () => {
      attachReplay()
      const options = replayIntegrationFn.mock.calls[0][0] as {
        beforeErrorSampling: (event: { tags?: Record<string, unknown> }) => boolean
      }
      return options.beforeErrorSampling
    }

    it('refuses to flush a replay for a planted-tag warning', () => {
      expect(
        samplingFor()({ tags: { error_type: 'planted_affiliate_tag' } })
      ).toBe(false)
    })

    it.each([
      ['a real error class', { tags: { error_type: 'rate_limit_exhausted' } }],
      ['an untagged event', {}],
      ['an event with no error_type', { tags: { runtime: 'browser' } }],
    ])('still flushes for %s', (_label, event) => {
      expect(samplingFor()(event)).toBe(true)
    })
  })
})
