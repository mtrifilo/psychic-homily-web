import { describe, it, expect, vi, beforeEach } from 'vitest'
import { type ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CreateCollectionForm } from './CreateCollectionForm'

// Admin user so the per-tier cap banner / submit-disable is skipped here
// (cap behavior is the catalog model's concern, not this refactor's).
const mockUseAuthContext = vi.fn(() => ({
  user: { id: '1', is_admin: true, user_tier: 'local_ambassador' },
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

const mockCreateMutateAsync = vi.fn()
const mockBulkAddMutateAsync = vi.fn()
vi.mock('../hooks', () => ({
  useCreateCollection: () => ({
    mutateAsync: mockCreateMutateAsync,
    isPending: false,
    error: null as Error | null,
  }),
  useBulkAddCollectionItems: () => ({
    mutateAsync: mockBulkAddMutateAsync,
    isPending: false,
    error: null as Error | null,
  }),
  useMyCollections: () => ({ data: { collections: [] }, isLoading: false }),
}))

// Stub the lazy MarkdownEditor (avoids the marked/dompurify load); keep the
// id so the form's <label htmlFor> association resolves.
vi.mock('./MarkdownEditorLazy', () => ({
  MarkdownEditor: ({
    id,
    value,
    onChange,
  }: {
    id: string
    value: string
    onChange: (v: string) => void
  }) => (
    <textarea id={id} value={value} onChange={(e) => onChange(e.target.value)} />
  ),
}))

// Stub the picker but surface its staged-items so we can assert the
// create-from-entity pre-fill (PSY-961).
vi.mock('./AddItemsPicker', () => ({
  AddItemsPicker: ({ stagedItems }: { stagedItems: { name: string }[] }) => (
    <div data-testid="staged-names">
      {stagedItems.map((s) => s.name).join(', ')}
    </div>
  ),
}))

vi.mock('next/link', () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}))

describe('CreateCollectionForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuthContext.mockReturnValue({
      user: { id: '1', is_admin: true, user_tier: 'local_ambassador' },
    })
  })

  it('renders with Public checked + Collaborative unchecked by default', () => {
    render(<CreateCollectionForm onSuccess={vi.fn()} />)
    expect(screen.getByLabelText('Title')).toBeInTheDocument()
    expect((screen.getByLabelText('Public') as HTMLInputElement).checked).toBe(
      true
    )
    expect(
      (screen.getByLabelText('Collaborative') as HTMLInputElement).checked
    ).toBe(false)
  })

  it('renders the Cover image URL field with the create-only helper text', () => {
    render(<CreateCollectionForm onSuccess={vi.fn()} />)
    expect(
      screen.getByTestId('create-cover-image-url-input')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Paste a direct image URL (e.g. Bandcamp art).')
    ).toBeInTheDocument()
  })

  it('pre-fills the staged list from initialStagedItems (create-from-entity)', () => {
    render(
      <CreateCollectionForm
        onSuccess={vi.fn()}
        initialStagedItems={[
          {
            entityType: 'artist',
            entityId: 42,
            name: 'Amyl and The Sniffers',
            subtitle: null,
          },
        ]}
      />
    )
    expect(screen.getByTestId('staged-names')).toHaveTextContent(
      'Amyl and The Sniffers'
    )
  })

  it('creates the collection then bulk-adds the staged items, then calls onSuccess', async () => {
    mockCreateMutateAsync.mockResolvedValue({ slug: 'amyl-collection' })
    mockBulkAddMutateAsync.mockResolvedValue({ added: [1], errors: [] })
    const onSuccess = vi.fn()
    const user = userEvent.setup()

    render(
      <CreateCollectionForm
        onSuccess={onSuccess}
        initialStagedItems={[
          { entityType: 'artist', entityId: 42, name: 'Amyl', subtitle: null },
        ]}
      />
    )

    await user.type(screen.getByLabelText('Title'), 'Amyl Collection')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Amyl Collection',
          is_public: true,
          collaborative: false,
        })
      )
    })
    // The pre-seeded entity is committed via the bulk-add endpoint.
    expect(mockBulkAddMutateAsync).toHaveBeenCalledWith({
      slug: 'amyl-collection',
      items: [{ entity_type: 'artist', entity_id: 42 }],
    })
    expect(onSuccess).toHaveBeenCalledWith('amyl-collection')
  })

  // PSY-585: cover-image URL payload contract (moved here from CollectionList).
  it('submits with cover_image_url when a valid URL is entered', async () => {
    mockCreateMutateAsync.mockResolvedValue({ slug: 's' })
    mockBulkAddMutateAsync.mockResolvedValue({ added: [], errors: [] })
    const user = userEvent.setup()
    render(<CreateCollectionForm onSuccess={vi.fn()} />)

    await user.type(screen.getByLabelText('Title'), 'My Picks')
    await user.type(
      screen.getByTestId('create-cover-image-url-input'),
      'https://example.com/cover.jpg'
    )
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'My Picks',
          cover_image_url: 'https://example.com/cover.jpg',
        })
      )
    })
  })

  it('omits cover_image_url from the payload when the field is empty', async () => {
    mockCreateMutateAsync.mockResolvedValue({ slug: 's' })
    mockBulkAddMutateAsync.mockResolvedValue({ added: [], errors: [] })
    const user = userEvent.setup()
    render(<CreateCollectionForm onSuccess={vi.fn()} />)

    await user.type(screen.getByLabelText('Title'), 'No Cover')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await waitFor(() => expect(mockCreateMutateAsync).toHaveBeenCalled())
    // JSON.stringify drops undefined keys → an empty cover URL never reaches
    // the wire body.
    const payload = mockCreateMutateAsync.mock.calls[0][0] as Record<
      string,
      unknown
    >
    expect(payload.cover_image_url).toBeUndefined()
  })

  it('disables submit for an invalid cover URL and never fires the mutation', async () => {
    const user = userEvent.setup()
    render(<CreateCollectionForm onSuccess={vi.fn()} />)

    await user.type(screen.getByLabelText('Title'), 'Bad URL Test')
    const coverInput = screen.getByTestId('create-cover-image-url-input')
    // `not-a-url` fails the WHATWG URL parser → validateCoverImageUrl errors.
    await user.type(coverInput, 'not-a-url')
    coverInput.blur()

    const submitButton = screen.getByRole('button', {
      name: /^create$/i,
    }) as HTMLButtonElement
    expect(submitButton.disabled).toBe(true)
    await user.click(submitButton)
    expect(mockCreateMutateAsync).not.toHaveBeenCalled()
  })
})
