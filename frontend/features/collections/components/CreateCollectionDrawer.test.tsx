import { describe, it, expect, vi, beforeEach } from 'vitest'
import { type ReactNode } from 'react'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  CreateCollectionDrawerProvider,
  useCreateCollectionDrawer,
} from './CreateCollectionDrawer'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Capture the form's onSuccess + the Sheet's onOpenChange so tests can drive
// the success / user-dismiss paths directly.
let capturedOnSuccess: ((slug?: string) => void) | undefined
let capturedOnOpenChange: ((open: boolean) => void) | undefined

// The drawer lazy-loads CreateCollectionForm via next/dynamic. Replace dynamic
// with a stub that surfaces the pre-fill + captures onSuccess, without pulling
// in the real form (+ AddItemsPicker) chunk.
vi.mock('next/dynamic', () => ({
  default: () =>
    function MockForm({
      initialStagedItems,
      onSuccess,
    }: {
      initialStagedItems?: { name: string }[]
      onSuccess?: (slug?: string) => void
    }) {
      capturedOnSuccess = onSuccess
      return (
        <div data-testid="create-form">
          form:{(initialStagedItems ?? []).map((s) => s.name).join(',')}
        </div>
      )
    },
}))

// Render the Sheet's content only when open (no portal/animation in jsdom);
// capture onOpenChange so a test can simulate a user dismiss (Esc/overlay).
vi.mock('@/components/ui/sheet', () => ({
  Sheet: ({
    open,
    onOpenChange,
    children,
  }: {
    open: boolean
    onOpenChange?: (open: boolean) => void
    children: ReactNode
  }) => {
    capturedOnOpenChange = onOpenChange
    return open ? <div data-testid="sheet">{children}</div> : null
  },
  SheetContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SheetHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SheetTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}))

function OpenButton({ name }: { name: string }) {
  const { openCreateDrawer } = useCreateCollectionDrawer()
  return (
    <button
      onClick={() =>
        openCreateDrawer({
          initialStagedItems: [
            { entityType: 'artist', entityId: 1, name, subtitle: null },
          ],
        })
      }
    >
      open
    </button>
  )
}

describe('CreateCollectionDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    capturedOnSuccess = undefined
    capturedOnOpenChange = undefined
  })

  it('throws when useCreateCollectionDrawer is used outside the provider', () => {
    function Bare() {
      useCreateCollectionDrawer()
      return null
    }
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Bare />)).toThrow(/CreateCollectionDrawerProvider/)
    spy.mockRestore()
  })

  it('opens the drawer with the pre-filled form when openCreateDrawer is called', async () => {
    const user = userEvent.setup()
    render(
      <CreateCollectionDrawerProvider>
        <OpenButton name="Amyl and The Sniffers" />
      </CreateCollectionDrawerProvider>
    )

    // Closed initially — the form (and its lazy chunk) is not mounted.
    expect(screen.queryByTestId('create-form')).not.toBeInTheDocument()

    await user.click(screen.getByText('open'))

    expect(await screen.findByTestId('create-form')).toHaveTextContent(
      'Amyl and The Sniffers'
    )
  })

  it('navigates to the new collection on a successful create', async () => {
    const user = userEvent.setup()
    render(
      <CreateCollectionDrawerProvider>
        <OpenButton name="Amyl" />
      </CreateCollectionDrawerProvider>
    )
    await user.click(screen.getByText('open'))
    await screen.findByTestId('create-form')

    // The form reports success (the user did NOT dismiss the drawer).
    act(() => capturedOnSuccess?.('amyl-collection'))
    expect(mockPush).toHaveBeenCalledWith('/collections/amyl-collection')
  })

  it('skips navigation when the user dismissed the drawer before create resolved', async () => {
    const user = userEvent.setup()
    render(
      <CreateCollectionDrawerProvider>
        <OpenButton name="Amyl" />
      </CreateCollectionDrawerProvider>
    )
    await user.click(screen.getByText('open'))
    await screen.findByTestId('create-form')

    // User dismisses (Esc / overlay) while the create is still in flight…
    act(() => capturedOnOpenChange?.(false))
    // …then the create resolves — the user must not be yanked away.
    act(() => capturedOnSuccess?.('amyl-collection'))
    expect(mockPush).not.toHaveBeenCalled()
  })
})
