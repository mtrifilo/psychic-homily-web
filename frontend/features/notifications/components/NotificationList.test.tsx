import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NotificationList } from './NotificationList'
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

  it('falls back to "Someone" when commenter_name is missing', () => {
    render(
      <NotificationList
        entries={[commentReply({ commenter_name: undefined })]}
      />
    )
    expect(screen.getByText('Someone')).toBeInTheDocument()
  })
})
