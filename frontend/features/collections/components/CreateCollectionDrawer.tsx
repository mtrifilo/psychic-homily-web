'use client'

import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import dynamic from 'next/dynamic'
import { useRouter } from 'next/navigation'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Loader2 } from 'lucide-react'
import type { StagedCollectionItem } from './AddItemsPicker'

// PSY-961: lazy-load the form. The provider is mounted app-wide (root layout),
// so eagerly importing the form would pull AddItemsPicker + its extraction/
// search deps into the global client chunk. `dynamic(ssr:false)` keeps that
// weight in a separate chunk fetched only when the drawer first opens — per
// pattern_turbopack_shared_chunk.
const CreateCollectionForm = dynamic(
  () =>
    import('./CreateCollectionForm').then((m) => m.CreateCollectionForm),
  {
    ssr: false,
    loading: () => (
      <div className="flex items-center justify-center py-10">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

interface OpenCreateDrawerOptions {
  /** Pre-seed the staged list — create-from-entity passes the entity as item 1. */
  initialStagedItems?: StagedCollectionItem[]
}

interface CreateCollectionDrawerContextValue {
  openCreateDrawer: (options?: OpenCreateDrawerOptions) => void
}

const CreateCollectionDrawerContext =
  createContext<CreateCollectionDrawerContextValue | null>(null)

/**
 * App-level "create a collection" drawer (PSY-961 / PSY-893 D4). Mounted once
 * near the root so any surface — the /collections "Create Collection" button or
 * the AddToCollectionButton popover's "Create … with {entity}" CTA — can open
 * an in-place Create drawer without navigating away. Call `openCreateDrawer()`
 * from `useCreateCollectionDrawer()`.
 */
export function CreateCollectionDrawerProvider({
  children,
}: {
  children: ReactNode
}) {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [initialStagedItems, setInitialStagedItems] = useState<
    StagedCollectionItem[]
  >([])
  // If the user dismisses the drawer (Esc / overlay / X) WHILE a create is
  // still in flight, skip the post-success navigation so they aren't yanked
  // off the page they just backed out of. Radix's onOpenChange fires only on
  // a user-driven close (not on our own setOpen(false)), so it cleanly marks
  // intent. The collection is still created — the request already fired.
  const userDismissedRef = useRef(false)

  const openCreateDrawer = useCallback(
    (options?: OpenCreateDrawerOptions) => {
      userDismissedRef.current = false
      setInitialStagedItems(options?.initialStagedItems ?? [])
      setOpen(true)
    },
    []
  )

  return (
    <CreateCollectionDrawerContext.Provider value={{ openCreateDrawer }}>
      {children}
      <Sheet
        open={open}
        onOpenChange={(next) => {
          if (!next) userDismissedRef.current = true
          setOpen(next)
        }}
      >
        <SheetContent
          side="right"
          className="w-full sm:max-w-xl flex flex-col overflow-y-auto"
        >
          <SheetHeader>
            <SheetTitle>Create Collection</SheetTitle>
          </SheetHeader>
          <div className="px-4 pb-4">
            {/* Only mount the form while open: the lazy chunk + its queries
                load on first open (not on every page), and unmounting on
                close resets the form so each open starts fresh with the
                current pre-fill. */}
            {open && (
              <CreateCollectionForm
                initialStagedItems={initialStagedItems}
                onSuccess={(newSlug) => {
                  setOpen(false)
                  if (newSlug && !userDismissedRef.current) {
                    router.push(`/collections/${newSlug}`)
                  }
                }}
                onCancel={() => setOpen(false)}
              />
            )}
          </div>
        </SheetContent>
      </Sheet>
    </CreateCollectionDrawerContext.Provider>
  )
}

export function useCreateCollectionDrawer(): CreateCollectionDrawerContextValue {
  const ctx = useContext(CreateCollectionDrawerContext)
  if (!ctx) {
    throw new Error(
      'useCreateCollectionDrawer must be used within a CreateCollectionDrawerProvider'
    )
  }
  return ctx
}
