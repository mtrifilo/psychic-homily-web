'use client'

import {
  createContext,
  useCallback,
  useContext,
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
  // Bumped on every open so the form remounts fresh (new pre-fill, cleared
  // fields) even if Radix keeps the content mounted between rapid reopens.
  const [openNonce, setOpenNonce] = useState(0)

  const openCreateDrawer = useCallback(
    (options?: OpenCreateDrawerOptions) => {
      setInitialStagedItems(options?.initialStagedItems ?? [])
      setOpenNonce((n) => n + 1)
      setOpen(true)
    },
    []
  )

  return (
    <CreateCollectionDrawerContext.Provider value={{ openCreateDrawer }}>
      {children}
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          side="right"
          className="w-full sm:max-w-xl flex flex-col overflow-y-auto"
        >
          <SheetHeader>
            <SheetTitle>Create Collection</SheetTitle>
          </SheetHeader>
          <div className="px-4 pb-4">
            {/* Only mount the form while open so the lazy chunk + its queries
                load on first open, not on every page. */}
            {open && (
              <CreateCollectionForm
                key={openNonce}
                initialStagedItems={initialStagedItems}
                onSuccess={(newSlug) => {
                  setOpen(false)
                  if (newSlug) {
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
