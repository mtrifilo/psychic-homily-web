import { describe, it, expect, vi } from 'vitest'
import { type ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  CreateCollectionDrawerProvider,
  useCreateCollectionDrawer,
} from './CreateCollectionDrawer'

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

// The drawer lazy-loads CreateCollectionForm via next/dynamic. Replace dynamic
// with a stub component that surfaces the pre-fill so we can assert it without
// pulling in the real form (+ AddItemsPicker) chunk.
vi.mock('next/dynamic', () => ({
  default: () =>
    function MockForm({
      initialStagedItems,
    }: {
      initialStagedItems?: { name: string }[]
    }) {
      return (
        <div data-testid="create-form">
          form:{(initialStagedItems ?? []).map((s) => s.name).join(',')}
        </div>
      )
    },
}))

// Render the Sheet's content only when open (no portal/animation in jsdom).
vi.mock('@/components/ui/sheet', () => ({
  Sheet: ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? <div data-testid="sheet">{children}</div> : null,
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
})
