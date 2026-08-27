import { describe, it, expect, vi, beforeEach } from 'vitest'

// PSY-1896 links this path as the "manage" CTA of every artist show-alert
// email, so where it lands is a product contract, not an implementation
// detail. It pointed at the criteria-filter builder until PSY-1905 moved
// notification-preference ownership to the Alerts matrix.

const mockRedirect = vi.fn()

vi.mock('next/navigation', () => ({
  redirect: (path: string) => mockRedirect(path),
}))

import LegacyNotificationSettingsRedirect from './page'
import { ALERTS_HREF } from '@/components/shared/followAlertChoices'

describe('/settings/notifications', () => {
  beforeEach(() => {
    mockRedirect.mockReset()
  })

  it('redirects to the Alerts card, not the Custom alerts builder', () => {
    LegacyNotificationSettingsRedirect()

    expect(mockRedirect).toHaveBeenCalledWith(ALERTS_HREF)
    expect(mockRedirect).not.toHaveBeenCalledWith(
      '/settings/notification-filters'
    )
  })

  // The target is built from the shared anchor constant, so renaming the
  // anchor cannot leave this redirect pointing at a fragment that is gone.
  it('targets the anchored matrix', () => {
    LegacyNotificationSettingsRedirect()
    expect(mockRedirect).toHaveBeenCalledWith(
      expect.stringContaining('#alerts')
    )
  })
})
