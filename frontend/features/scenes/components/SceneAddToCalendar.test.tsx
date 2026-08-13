import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SceneAddToCalendar, sceneCalendarIcsUrl } from './SceneAddToCalendar'

// vitest.config.mts pins NEXT_PUBLIC_API_URL to http://localhost:8080, so
// API_BASE_URL is deterministic here.
const ICS_URL = 'http://localhost:8080/scenes/phoenix-az/calendar.ics'

describe('sceneCalendarIcsUrl', () => {
  it('builds an absolute URL against the API origin', () => {
    expect(sceneCalendarIcsUrl('phoenix-az')).toBe(ICS_URL)
  })

  it('encodes hostile slugs so they cannot escape the path', () => {
    expect(sceneCalendarIcsUrl('a/../../evil?x=1')).toBe(
      'http://localhost:8080/scenes/a%2F..%2F..%2Fevil%3Fx%3D1/calendar.ics'
    )
  })
})

describe('SceneAddToCalendar', () => {
  async function openPopover(slug = 'phoenix-az') {
    const user = userEvent.setup()
    render(<SceneAddToCalendar slug={slug} />)
    await user.click(screen.getByRole('button', { name: /subscribe: \.ics/i }))
    return user
  }

  it('renders both calendar actions with correct targets once opened', async () => {
    await openPopover()

    expect(
      screen.getByRole('link', { name: /apple \/ outlook/i })
    ).toHaveAttribute('href', ICS_URL)
    const google = screen.getByRole('link', { name: /google calendar/i })
    expect(google.getAttribute('href')).toContain(
      'https://calendar.google.com/calendar/r?cid='
    )
    expect(google.getAttribute('href')).toContain(encodeURIComponent('webcal://'))
    expect(google).toHaveAttribute('target', '_blank')
  })

  it('is never auth-gated: the trigger renders with no session', async () => {
    render(<SceneAddToCalendar slug="phoenix-az" />)
    expect(
      screen.getByRole('button', { name: /subscribe: \.ics/i })
    ).toBeInTheDocument()
  })
})
