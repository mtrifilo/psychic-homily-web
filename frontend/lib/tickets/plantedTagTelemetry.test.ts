import { describe, it, expect, vi, beforeEach } from 'vitest'

const captureMessage = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureMessage: (...args: unknown[]) => captureMessage(...args),
}))

const TAG = {
  param: 'irmp',
  host: 'www.ticketweb.com',
  matchesConfiguredPartner: false,
}

// Sampling is a volume control, not behaviour under test: every assertion here
// is about WHICH reports are eligible, so the roll is pinned to always pass.
// The sampling test below is the one place it is exercised.
function alwaysSample() {
  return vi.spyOn(Math, 'random').mockReturnValue(0)
}
const SHOW_42 = { entityType: 'show' as const, entityId: 42, tag: TAG }

/**
 * A fresh module per test, rather than a `reset...ForTest` export.
 *
 * The dedupe state IS the volume control on an event class an untrusted user
 * can trigger, so a public handle for clearing it would ship in the client
 * bundle. Re-importing gives the isolation without putting one there.
 */
async function loadTelemetry() {
  vi.resetModules()
  sessionStorage.clear()
  alwaysSample()
  return import('./plantedTagTelemetry')
}

type CaptureOptions = {
  level: string
  tags: Record<string, unknown>
  extra: Record<string, unknown>
}

beforeEach(() => {
  captureMessage.mockClear()
  vi.restoreAllMocks()
})

describe('reportPlantedTicketTag', () => {
  it('reports a warning naming the host and the parameter', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    reportPlantedTicketTag(SHOW_42)

    expect(captureMessage).toHaveBeenCalledTimes(1)
    const [message, options] = captureMessage.mock.calls[0] as [
      string,
      CaptureOptions,
    ]
    expect(message).toMatch(/planted/i)
    expect(options.level).toBe('warning')
    expect(options.tags).toMatchObject({
      error_type: 'planted_affiliate_tag',
      entity_type: 'show',
      ticket_host: 'www.ticketweb.com',
      affiliate_param: 'irmp',
    })
    expect(options.extra).toMatchObject({ entityId: 42 })
  })

  // The partner ID is a third party's account identifier and the rest of the
  // URL is contributor text. Neither may leave the process.
  //
  // Asserts the payload's SHAPE, not the absence of strings that were never
  // passed in: a test that only looked for a partner ID it never supplied
  // would pass for an implementation that dumped its whole argument into
  // `extra`. Widening PlantedTicketTag (the obvious next move is carrying the
  // tag's value, to tell whose it is) must fail HERE.
  it('ships exactly the agreed fields and nothing else', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    reportPlantedTicketTag(SHOW_42)

    const [, options] = captureMessage.mock.calls[0] as [string, CaptureOptions]
    expect(Object.keys(options.tags).sort()).toEqual([
      'affiliate_param',
      'entity_type',
      'error_type',
      'matches_configured_partner',
      'runtime',
      'ticket_host',
    ])
    expect(Object.keys(options.extra).sort()).toEqual(['entityId', 'renderedAs'])
  })

  // A hostile extra field on the tag must not ride along, whatever a future
  // caller puts there.
  it('does not propagate fields beyond param and host', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    reportPlantedTicketTag({
      entityType: 'show',
      entityId: 42,
      tag: {
        ...TAG,
        value: '9999999',
        url: 'https://www.ticketweb.com/e/2?irmp=9999999',
      } as unknown as typeof TAG,
    })

    const serialized = JSON.stringify(captureMessage.mock.calls[0])
    expect(serialized).not.toContain('9999999')
    expect(serialized).not.toContain('/e/2')
    expect(serialized).not.toMatch(/https?:\/\//)
  })

  // A planted tag is a property of a stored row, so every viewer and every
  // re-render sees the same one. Per-occurrence reporting would turn one bad
  // row into unbounded identical events.
  it('reports a given row and tag only once', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    for (let i = 0; i < 5; i++) reportPlantedTicketTag(SHOW_42)
    expect(captureMessage).toHaveBeenCalledTimes(1)
  })

  // The trigger is a string an untrusted contributor stores, and a reload
  // resets module memory. Without a store that outlives the document, a
  // scripted reload loop mints one event per request.
  it('stays deduped across a module reload within the session', async () => {
    const first = await loadTelemetry()
    first.reportPlantedTicketTag(SHOW_42)
    expect(captureMessage).toHaveBeenCalledTimes(1)

    // A reload: fresh module, same sessionStorage.
    vi.resetModules()
    const second = await import('./plantedTagTelemetry')
    second.reportPlantedTicketTag(SHOW_42)
    expect(captureMessage).toHaveBeenCalledTimes(1)
  })

  it('still reports when sessionStorage throws', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    const getItem = vi
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation(() => {
        throw new Error('disabled')
      })
    const setItem = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new Error('quota')
      })
    try {
      reportPlantedTicketTag(SHOW_42)
      expect(captureMessage).toHaveBeenCalledTimes(1)
      // In-memory dedupe still carries it within the document.
      reportPlantedTicketTag(SHOW_42)
      expect(captureMessage).toHaveBeenCalledTimes(1)
    } finally {
      getItem.mockRestore()
      setItem.mockRestore()
    }
  })

  // Every keyed bound is keyed on a dimension the attacker picks, so the only
  // real backstop is one keyed on nothing.
  it('stops at a global ceiling however many distinct rows are seen', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    for (let id = 0; id < 500; id++) {
      reportPlantedTicketTag({ entityType: 'show', entityId: id, tag: TAG })
    }
    expect(captureMessage.mock.calls.length).toBeLessThanOrEqual(20)
  })

  it.each([
    ['a different entity', { entityType: 'show' as const, entityId: 43, tag: TAG }],
    [
      'a different entity type',
      { entityType: 'festival' as const, entityId: 42, tag: TAG },
    ],
    [
      'a different host',
      { entityType: 'show' as const, entityId: 42, tag: { ...TAG, host: 'dice.fm' } },
    ],
  ])('still reports %s', async (_label, second) => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    reportPlantedTicketTag(SHOW_42)
    reportPlantedTicketTag(second)
    expect(captureMessage).toHaveBeenCalledTimes(2)
  })

  // Volume otherwise scales with unique visitors to a page a contributor
  // chooses. A losing roll must also leave the row UNMARKED, so the next
  // visitor asks again and a real planted tag still surfaces.
  it('drops a report that loses the sampling roll, and re-asks next time', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    const random = vi.spyOn(Math, 'random').mockReturnValue(0.99)
    reportPlantedTicketTag(SHOW_42)
    expect(captureMessage).not.toHaveBeenCalled()

    random.mockReturnValue(0)
    reportPlantedTicketTag(SHOW_42)
    expect(captureMessage).toHaveBeenCalledTimes(1)
  })

  // This runs from a render effect on a page whose job is showing a reader
  // where to buy a ticket. A telemetry fault must never become that page's
  // problem.
  it('never throws when Sentry does', async () => {
    const { reportPlantedTicketTag } = await loadTelemetry()
    captureMessage.mockImplementationOnce(() => {
      throw new Error('transport down')
    })
    expect(() => reportPlantedTicketTag(SHOW_42)).not.toThrow()
  })
})
