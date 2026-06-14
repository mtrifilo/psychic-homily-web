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

    expect(replayIntegrationFn).toHaveBeenCalledWith({
      maskAllText: true,
      blockAllMedia: true,
    })
    expect(Sentry.addIntegration).toHaveBeenCalledWith({
      name: 'Replay',
      options: { maskAllText: true, blockAllMedia: true },
    })
  })
})
