import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  EarlierDivider,
  NotificationList,
  partitionNotificationsByRead,
} from './NotificationList'
import type { NotificationLogEntry } from '../types'

function commentReply(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    id: 1,
    entity_type: 'comment_reply',
    entity_id: 100,
    channel: 'in_app',
    sent_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
    read_at: null,
    commenter_name: 'alice',
    commenter_username: 'alice',
    comment_excerpt: 'this is the reply body excerpt',
    comment_url: 'https://example.com/shows/the-show#comment-100',
    comment_entity_type: 'show',
    comment_entity_id: 42,
    comment_entity_name: 'The Show',
    ...overrides,
  }
}

function commentMention(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    ...commentReply(),
    id: 2,
    entity_type: 'comment_mention',
    commenter_name: 'bob',
    commenter_username: 'bob',
    comment_excerpt: 'hey @you check this',
    ...overrides,
  }
}

function showFilter(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    id: 3,
    filter_id: 7,
    filter_name: 'My Filter',
    entity_type: 'show',
    entity_id: 200,
    channel: 'email',
    sent_at: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
    read_at: new Date().toISOString(),
    ...overrides,
  }
}

function requestFulfillment(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    id: 5,
    entity_type: 'request_fulfillment_proposed',
    entity_id: 300,
    channel: 'in_app',
    sent_at: new Date(Date.now() - 10 * 60 * 1000).toISOString(),
    read_at: null,
    request_title: 'Add Local Band XYZ',
    request_url: 'https://example.com/requests/300',
    ...overrides,
  }
}

function artistShowAlert(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    id: 6,
    entity_type: 'artist_show_alert',
    entity_id: 400,
    subject_entity_id: 55,
    channel: 'in_app',
    sent_at: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    read_at: null,
    alert_artist_name: 'Oneida',
    alert_show_title: 'Oneida at Valley Bar',
    alert_show_url: 'https://example.com/shows/oneida-valley-bar',
    ...overrides,
  }
}

function venueShowAlert(overrides: Partial<NotificationLogEntry> = {}): NotificationLogEntry {
  return {
    id: 7,
    entity_type: 'venue_show_alert',
    // A VENUE id, not a show id, and no subject_entity_id: the followed entity
    // and the row's subject are the same venue.
    entity_id: 500,
    channel: 'in_app',
    sent_at: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    read_at: null,
    alert_bucket: '2026-08-24',
    alert_venue_name: 'Valley Bar',
    alert_venue_url: 'https://example.com/venues/valley-bar',
    alert_show_count: 3,
    alert_show_summary:
      'Sat Aug 29 Oneida · Fri Sep 5 Chat Pile · Sat Sep 6 Turnstile',
    ...overrides,
  }
}

describe('NotificationList', () => {
  it('renders empty state when no entries', () => {
    render(<NotificationList entries={[]} />)
    expect(screen.getByText(/all caught up/i)).toBeInTheDocument()
  })

  it('renders a comment_reply row with commenter name + excerpt + entity', () => {
    render(<NotificationList entries={[commentReply()]} />)
    expect(screen.getByText('alice')).toBeInTheDocument()
    // "replied on" is one whitespace-joined span fragment.
    expect(screen.getByText(/replied on/)).toBeInTheDocument()
    expect(screen.getByText('The Show')).toBeInTheDocument()
    expect(
      screen.getByText('this is the reply body excerpt')
    ).toBeInTheDocument()
  })

  it('renders a comment_mention row with "mentioned you" verb', () => {
    render(<NotificationList entries={[commentMention()]} />)
    expect(screen.getByText(/mentioned you on/)).toBeInTheDocument()
    expect(screen.getByText('bob')).toBeInTheDocument()
  })

  it('uses comment_url as the deep-link target for comment rows', () => {
    render(<NotificationList entries={[commentReply()]} />)
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute(
      'href',
      'https://example.com/shows/the-show#comment-100'
    )
  })

  it('renders show-filter rows with filter_name', () => {
    render(<NotificationList entries={[showFilter()]} />)
    expect(screen.getByText('My Filter')).toBeInTheDocument()
    expect(screen.getByText(/new match for/i)).toBeInTheDocument()
  })

  it('renders a request_fulfillment_proposed row with title + approve/reject prompt', () => {
    render(<NotificationList entries={[requestFulfillment()]} />)
    expect(screen.getByText(/a fulfillment was proposed for/i)).toBeInTheDocument()
    expect(screen.getByText('Add Local Band XYZ')).toBeInTheDocument()
    expect(screen.getByText(/review it to approve or reject/i)).toBeInTheDocument()
  })

  it('uses request_url as the deep-link target for request rows', () => {
    render(<NotificationList entries={[requestFulfillment()]} />)
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      'https://example.com/requests/300'
    )
  })

  it('falls back to "your request" + /requests when request fields are missing', () => {
    render(
      <NotificationList
        entries={[requestFulfillment({ request_title: undefined, request_url: undefined })]}
      />
    )
    expect(screen.getByText('your request')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '/requests')
  })

  it('marks unread rows visually (Unread label) and read rows without', () => {
    render(
      <NotificationList entries={[commentReply(), commentReply({ id: 4, read_at: new Date().toISOString() })]} />
    )
    expect(screen.getAllByLabelText('Unread')).toHaveLength(1)
  })

  it('fires onItemClick when a row is clicked', async () => {
    const onItemClick = vi.fn()
    const entry = commentReply()
    const user = userEvent.setup()
    render(<NotificationList entries={[entry]} onItemClick={onItemClick} />)
    await user.click(screen.getByRole('link'))
    expect(onItemClick).toHaveBeenCalledWith(entry)
  })

  it('renders a [mark read] affordance on unread rows when onMarkRead is provided', async () => {
    const onMarkRead = vi.fn()
    const onItemClick = vi.fn()
    const entry = commentReply()
    const user = userEvent.setup()
    render(
      <NotificationList
        entries={[entry]}
        onItemClick={onItemClick}
        onMarkRead={onMarkRead}
      />
    )
    await user.click(screen.getByRole('button', { name: '[mark read]' }))
    expect(onMarkRead).toHaveBeenCalledWith(entry)
    // stopPropagation keeps the row's navigate-click handler out of it.
    expect(onItemClick).not.toHaveBeenCalled()
  })

  it('does not render [mark read] on read rows or without onMarkRead', () => {
    const { rerender } = render(
      <NotificationList
        entries={[commentReply({ read_at: new Date().toISOString() })]}
        onMarkRead={vi.fn()}
      />
    )
    expect(screen.queryByText('[mark read]')).not.toBeInTheDocument()
    rerender(<NotificationList entries={[commentReply()]} />)
    expect(screen.queryByText('[mark read]')).not.toBeInTheDocument()
  })

  it('dims rows when dimmed is set', () => {
    render(<NotificationList entries={[showFilter()]} dimmed />)
    expect(screen.getByRole('link')).toHaveClass('opacity-60')
  })

  it('falls back to "Someone" when commenter_name is missing', () => {
    render(
      <NotificationList
        entries={[commentReply({ commenter_name: undefined })]}
      />
    )
    expect(screen.getByText('Someone')).toBeInTheDocument()
  })

  // PSY-1896: the artist new-show alert row.
  it('names the followed artist and links the show for an artist show alert', () => {
    render(<NotificationList entries={[artistShowAlert()]} />)
    expect(screen.getByText('Oneida')).toBeInTheDocument()
    expect(screen.getByText('announced a show')).toBeInTheDocument()
    expect(screen.getByText('Oneida at Valley Bar')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      'https://example.com/shows/oneida-valley-bar'
    )
  })

  it('degrades an artist show alert whose enrichment came back empty', () => {
    // A merged artist or a deleted show leaves the fields blank. The
    // notification still happened, so the row must stay usable rather than
    // rendering a bare entity_type linked to nowhere.
    render(
      <NotificationList
        entries={[
          artistShowAlert({
            alert_artist_name: undefined,
            alert_show_title: undefined,
            alert_show_url: undefined,
          }),
        ]}
      />
    )
    expect(screen.getByText('An artist you follow')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '/shows')
    expect(screen.queryByText('artist_show_alert')).not.toBeInTheDocument()
  })

  it('marks an unread artist show alert read from the row affordance', async () => {
    const onMarkRead = vi.fn()
    render(
      <NotificationList entries={[artistShowAlert()]} onMarkRead={onMarkRead} />
    )
    await userEvent.click(screen.getByText('[mark read]'))
    expect(onMarkRead).toHaveBeenCalledTimes(1)
  })

  // PSY-1895: the venue new-show alert row. Coalesced, so it counts shows and
  // links the VENUE rather than any one date.
  it('names the venue, counts the shows, and links the venue', () => {
    render(<NotificationList entries={[venueShowAlert()]} />)
    expect(screen.getByText('Valley Bar')).toBeInTheDocument()
    expect(screen.getByText('added 3 new shows')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Sat Aug 29 Oneida · Fri Sep 5 Chat Pile · Sat Sep 6 Turnstile'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      'https://example.com/venues/valley-bar'
    )
  })

  it('singularizes a venue alert covering one show', () => {
    render(
      <NotificationList
        entries={[
          venueShowAlert({
            alert_show_count: 1,
            alert_show_summary: 'Fri Sep 5 Chat Pile',
          }),
        ]}
      />
    )
    expect(screen.getByText('added a new show')).toBeInTheDocument()
  })

  it('degrades a venue show alert whose enrichment came back empty', () => {
    // A merged or deleted venue leaves the fields blank. The notification still
    // happened, so the row stays usable rather than rendering a bare
    // entity_type linked to nowhere — and the sentence stays grammatical even
    // with no count.
    render(
      <NotificationList
        entries={[
          venueShowAlert({
            alert_venue_name: undefined,
            alert_venue_url: undefined,
            alert_show_count: undefined,
            alert_show_summary: undefined,
          }),
        ]}
      />
    )
    expect(screen.getByText('A venue you follow')).toBeInTheDocument()
    expect(screen.getByText('added a new show')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '/venues')
    expect(screen.queryByText('venue_show_alert')).not.toBeInTheDocument()
  })

  // The failure this guards is silent: entity_id on a venue alert is a VENUE
  // id, so falling through to the artist branch would build /shows/<venue id>.
  it('does not render a venue alert through the artist branch', () => {
    render(<NotificationList entries={[venueShowAlert()]} />)
    expect(screen.queryByText('announced a show')).not.toBeInTheDocument()
  })

  it('marks an unread venue show alert read from the row affordance', async () => {
    const onMarkRead = vi.fn()
    render(
      <NotificationList entries={[venueShowAlert()]} onMarkRead={onMarkRead} />
    )
    await userEvent.click(screen.getByText('[mark read]'))
    expect(onMarkRead).toHaveBeenCalledTimes(1)
  })
})

describe('partitionNotificationsByRead', () => {
  it('splits entries into unread and read groups, preserving order', () => {
    const a = commentReply({ id: 1 })
    const b = commentReply({ id: 2, read_at: new Date().toISOString() })
    const c = commentMention({ id: 3 })
    const { unread, read } = partitionNotificationsByRead([a, b, c])
    expect(unread.map(e => e.id)).toEqual([1, 3])
    expect(read.map(e => e.id)).toEqual([2])
  })
})

describe('EarlierDivider', () => {
  it('renders the EARLIER hairline label', () => {
    render(<EarlierDivider />)
    expect(screen.getByText('Earlier')).toBeInTheDocument()
  })
})
