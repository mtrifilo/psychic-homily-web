import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type {
  TagAliasListing,
  BulkAliasImportResult,
} from '../types'

const mockUseAllTagAliases = vi.fn()
const mockBulkMutate = vi.fn()
const mockUseBulkImport = vi.fn()

vi.mock('./useAdminTags', () => ({
  useAllTagAliases: (...args: unknown[]) => mockUseAllTagAliases(...args),
  useBulkImportAliases: () => mockUseBulkImport(),
}))

import { AliasListing, parseCSV } from './AliasListing'

function makeAlias(overrides: Partial<TagAliasListing> = {}): TagAliasListing {
  return {
    id: 1,
    alias: 'dnb',
    tag_id: 10,
    tag_name: 'drum-and-bass',
    tag_slug: 'drum-and-bass',
    tag_category: 'genre',
    tag_is_official: false,
    created_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('parseCSV', () => {
  it('parses comma-separated rows', () => {
    const rows = parseCSV('dnb,drum-and-bass\nhiphop,hip-hop')
    expect(rows).toEqual([
      { alias: 'dnb', canonical: 'drum-and-bass' },
      { alias: 'hiphop', canonical: 'hip-hop' },
    ])
  })

  it('parses tab-separated rows', () => {
    const rows = parseCSV('dnb\tdrum-and-bass\nhiphop\thip-hop')
    expect(rows).toEqual([
      { alias: 'dnb', canonical: 'drum-and-bass' },
      { alias: 'hiphop', canonical: 'hip-hop' },
    ])
  })

  it('skips a header row with "alias,canonical" columns', () => {
    const rows = parseCSV('alias,canonical\ndnb,drum-and-bass')
    expect(rows).toEqual([{ alias: 'dnb', canonical: 'drum-and-bass' }])
  })

  it('skips comment and blank lines', () => {
    const rows = parseCSV('# seed file\n\ndnb,drum-and-bass\n\n# end')
    expect(rows).toEqual([{ alias: 'dnb', canonical: 'drum-and-bass' }])
  })

  it('trims whitespace around fields', () => {
    const rows = parseCSV('  dnb  ,  drum-and-bass  ')
    expect(rows).toEqual([{ alias: 'dnb', canonical: 'drum-and-bass' }])
  })

  it('drops rows with fewer than 2 fields', () => {
    const rows = parseCSV('only-one-field\nhip,hop')
    expect(rows).toEqual([{ alias: 'hip', canonical: 'hop' }])
  })
})

describe('AliasListing — rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockBulkMutate.mockReset()
    mockUseBulkImport.mockReturnValue({
      mutate: mockBulkMutate,
      isPending: false,
    })
  })

  it('shows loading spinner while loading', () => {
    mockUseAllTagAliases.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })
    const { container } = renderWithProviders(<AliasListing />)
    expect(container.querySelector('.animate-spin')).toBeTruthy()
  })

  it('shows empty state when no aliases', () => {
    mockUseAllTagAliases.mockReturnValue({
      data: { aliases: [], total: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<AliasListing />)
    expect(screen.getByText(/no aliases found/i)).toBeInTheDocument()
  })

  it('renders each alias with the arrow and canonical link', () => {
    mockUseAllTagAliases.mockReturnValue({
      data: {
        aliases: [
          makeAlias({ id: 1, alias: 'dnb', tag_name: 'drum-and-bass', tag_slug: 'drum-and-bass' }),
          makeAlias({ id: 2, alias: 'hiphop', tag_name: 'hip-hop', tag_slug: 'hip-hop', tag_id: 11 }),
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<AliasListing />)

    expect(screen.getByText('dnb')).toBeInTheDocument()
    expect(screen.getByText('hiphop')).toBeInTheDocument()
    const canonical = screen.getByRole('link', { name: 'drum-and-bass' })
    expect(canonical).toHaveAttribute('href', '/tags/drum-and-bass')
  })

  it('shows official indicator on official canonical tags', () => {
    mockUseAllTagAliases.mockReturnValue({
      data: {
        aliases: [
          makeAlias({ id: 1, alias: 'dnb', tag_name: 'drum-and-bass', tag_is_official: true }),
        ],
        total: 1,
      },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<AliasListing />)
    expect(screen.getByRole('img', { name: 'Official tag' })).toBeInTheDocument()
  })

  it('debounces search and refetches with the query', async () => {
    mockUseAllTagAliases.mockReturnValue({
      data: { aliases: [], total: 0 },
      isLoading: false,
      error: null,
    })
    renderWithProviders(<AliasListing />)

    const search = screen.getByPlaceholderText(/search aliases/i)
    fireEvent.change(search, { target: { value: 'dnb' } })

    await waitFor(() => {
      const calls = mockUseAllTagAliases.mock.calls
      const lastCall = calls[calls.length - 1]?.[0]
      expect(lastCall?.search).toBe('dnb')
    })
  })
})

describe('AliasListing — bulk import dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAllTagAliases.mockReturnValue({
      data: { aliases: [], total: 0 },
      isLoading: false,
      error: null,
    })
    mockBulkMutate.mockReset()
    mockUseBulkImport.mockReturnValue({
      mutate: mockBulkMutate,
      isPending: false,
    })
  })

  it('parses pasted CSV and shows the row count', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AliasListing />)

    await user.click(screen.getByRole('button', { name: /bulk import/i }))

    const textarea = await screen.findByLabelText(/or paste rows/i)
    await user.type(textarea, 'dnb,drum-and-bass')

    expect(await screen.findByText(/parsed 1 row/i)).toBeInTheDocument()
  })

  it('disables submit when no rows parsed', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AliasListing />)

    await user.click(screen.getByRole('button', { name: /bulk import/i }))

    const submit = await screen.findByRole('button', { name: /import 0 rows/i })
    expect(submit).toBeDisabled()
  })

  it('submits parsed rows and shows success + skipped summary', async () => {
    const user = userEvent.setup()
    const result: BulkAliasImportResult = {
      imported: 1,
      skipped: [
        {
          row: 2,
          alias: 'bad',
          canonical: 'nonexistent',
          reason: "canonical tag 'nonexistent' not found",
        },
      ],
    }
    mockBulkMutate.mockImplementation((_items, opts) => opts.onSuccess(result))

    renderWithProviders(<AliasListing />)
    await user.click(screen.getByRole('button', { name: /bulk import/i }))

    const textarea = await screen.findByLabelText(/or paste rows/i)
    await user.type(textarea, 'dnb,drum-and-bass\nbad,nonexistent')

    const submit = await screen.findByRole('button', { name: /import 2 rows/i })
    await user.click(submit)

    expect(mockBulkMutate).toHaveBeenCalled()
    expect(
      await screen.findByText(/imported 1 alias\. skipped 1 row\./i)
    ).toBeInTheDocument()
    expect(screen.getByText(/rejected rows/i)).toBeInTheDocument()
    expect(screen.getByText(/row 2/i)).toBeInTheDocument()
    expect(screen.getByText(/canonical tag 'nonexistent' not found/i)).toBeInTheDocument()
  })
})
