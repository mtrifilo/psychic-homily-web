'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Loader2, AlertCircle, GripVertical } from 'lucide-react'
import {
  useOwnSections,
  useCreateSection,
  useUpdateSection,
  useDeleteSection,
  type ProfileSectionResponse,
} from '@/features/auth'

const MAX_SECTIONS = 3

function emptySlotHint(remaining: number): string {
  if (remaining === 1) {
    return 'One more slot — add a section to personalize your profile.'
  }
  if (remaining === MAX_SECTIONS) {
    return 'Add a section to personalize your profile.'
  }
  return `${remaining} more slots — add a section to personalize your profile.`
}

export function ProfileSectionsEditor() {
  const { data: sectionsData, isLoading } = useOwnSections()
  const createSection = useCreateSection()
  const updateSection = useUpdateSection()
  const deleteSection = useDeleteSection()

  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [editingSection, setEditingSection] = useState<ProfileSectionResponse | null>(null)
  const [deletingSection, setDeletingSection] = useState<ProfileSectionResponse | null>(null)

  // Create form state
  const [newTitle, setNewTitle] = useState('')
  const [newContent, setNewContent] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  // Edit form state
  const [editTitle, setEditTitle] = useState('')
  const [editContent, setEditContent] = useState('')
  const [editVisible, setEditVisible] = useState(true)
  const [editError, setEditError] = useState<string | null>(null)

  const sections = sectionsData?.sections || []
  const sortedSections = [...sections].sort((a, b) => a.position - b.position)
  const canAddMore = sections.length < MAX_SECTIONS
  const remainingSlots = MAX_SECTIONS - sections.length

  const openCreateDialog = () => {
    setNewTitle('')
    setNewContent('')
    setFormError(null)
    setCreateDialogOpen(true)
  }

  const handleCreate = () => {
    setFormError(null)
    if (!newTitle.trim()) {
      setFormError('Title is required')
      return
    }
    if (!newContent.trim()) {
      setFormError('Content is required')
      return
    }
    createSection.mutate(
      {
        title: newTitle.trim(),
        content: newContent.trim(),
        position: sections.length,
      },
      {
        onSuccess: () => {
          setCreateDialogOpen(false)
          setNewTitle('')
          setNewContent('')
          setFormError(null)
        },
        onError: (err) => setFormError(err.message),
      }
    )
  }

  const handleEdit = () => {
    if (!editingSection) return
    setEditError(null)
    if (!editTitle.trim()) {
      setEditError('Title is required')
      return
    }
    if (!editContent.trim()) {
      setEditError('Content is required')
      return
    }
    updateSection.mutate(
      {
        sectionId: editingSection.id,
        data: {
          title: editTitle.trim(),
          content: editContent.trim(),
          is_visible: editVisible,
        },
      },
      {
        onSuccess: () => {
          setEditingSection(null)
          setEditError(null)
        },
        onError: (err) => setEditError(err.message),
      }
    )
  }

  const handleDelete = () => {
    if (!deletingSection) return
    deleteSection.mutate(deletingSection.id, {
      onSuccess: () => setDeletingSection(null),
    })
  }

  const openEditDialog = (section: ProfileSectionResponse) => {
    setEditTitle(section.title)
    setEditContent(section.content)
    setEditVisible(section.is_visible)
    setEditError(null)
    setEditingSection(section)
  }

  if (isLoading) {
    return (
      <Card>
        <CardContent className="flex justify-center p-6">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <div>
      <Card>
        <CardContent className="p-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h3 className="text-sm font-semibold">Custom sections</h3>
              <p className="mt-1 text-xs text-muted-foreground">
                Prose blocks rendered after your bio on the public profile.
                Markdown supported.
              </p>
            </div>
            <div className="flex shrink-0 flex-col items-end gap-2">
              <p className="font-mono text-[11px] text-muted-foreground">
                {sections.length} / {MAX_SECTIONS} used
              </p>
              {canAddMore && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={openCreateDialog}
                  className="h-auto px-1 py-1 text-[11px] font-semibold"
                >
                  + Add section
                </Button>
              )}
            </div>
          </div>

          <div className="mt-5">
            <div className="divide-y divide-border">
            {sortedSections.map((section) => (
              <div
                key={section.id}
                className="flex items-start gap-3 py-4 first:pt-0"
              >
                {/* Presentational grip — reorder DnD is not wired on this surface. */}
                <span
                  className="mt-0.5 shrink-0 text-muted-foreground"
                  aria-hidden
                >
                  <GripVertical className="h-4 w-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h4 className="text-[13px] font-medium leading-none">
                      {section.title}
                    </h4>
                    {!section.is_visible && (
                      <span className="rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                        Hidden
                      </span>
                    )}
                  </div>
                  {/* ~1.5-line preview: clamp to 2 lines with a tight max-height. */}
                  <p className="mt-1.5 max-h-[1.875rem] overflow-hidden text-xs leading-snug text-muted-foreground [display:-webkit-box] [-webkit-box-orient:vertical] [-webkit-line-clamp:2]">
                    {section.content}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-3 pt-0.5">
                  <button
                    type="button"
                    onClick={() => openEditDialog(section)}
                    aria-label="Edit section"
                    className="text-xs font-medium text-primary hover:underline"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => setDeletingSection(section)}
                    aria-label="Delete section"
                    className="text-xs font-medium text-muted-foreground hover:underline"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
            </div>

            {canAddMore && (
              <div className="mt-4 rounded-md border border-dashed border-border px-3.5 py-4">
                <p className="text-xs text-muted-foreground">
                  {emptySlotHint(remainingSlots)}
                </p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Create Dialog */}
      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Profile Section</DialogTitle>
            <DialogDescription>
              Add a custom section to your profile. This will be visible to others based on your privacy settings.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {formError && (
              <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 shrink-0" />
                <span>{formError}</span>
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="section-title">Title</Label>
              <Input
                id="section-title"
                value={newTitle}
                onChange={e => setNewTitle(e.target.value)}
                placeholder="e.g., About Me, Favorite Genres"
                maxLength={100}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="section-content">Content</Label>
              <Textarea
                id="section-content"
                value={newContent}
                onChange={e => setNewContent(e.target.value)}
                placeholder="Write something about yourself..."
                rows={5}
                maxLength={2000}
              />
              <p className="text-right text-xs text-muted-foreground">
                {newContent.length}/2000
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setCreateDialogOpen(false)}
              disabled={createSection.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={createSection.isPending}
            >
              {createSection.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Add Section
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={!!editingSection} onOpenChange={() => setEditingSection(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Section</DialogTitle>
            <DialogDescription>
              Update this profile section.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {editError && (
              <div className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 shrink-0" />
                <span>{editError}</span>
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="edit-section-title">Title</Label>
              <Input
                id="edit-section-title"
                value={editTitle}
                onChange={e => setEditTitle(e.target.value)}
                maxLength={100}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-section-content">Content</Label>
              <Textarea
                id="edit-section-content"
                value={editContent}
                onChange={e => setEditContent(e.target.value)}
                rows={5}
                maxLength={2000}
              />
              <p className="text-right text-xs text-muted-foreground">
                {editContent.length}/2000
              </p>
            </div>
            <div className="flex items-center justify-between">
              <Label htmlFor="edit-section-visible" className="text-sm">
                Visible on profile
              </Label>
              <Switch
                id="edit-section-visible"
                checked={editVisible}
                onCheckedChange={setEditVisible}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingSection(null)}
              disabled={updateSection.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={handleEdit}
              disabled={updateSection.isPending}
            >
              {updateSection.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Save Changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={!!deletingSection} onOpenChange={() => setDeletingSection(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Section</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &ldquo;{deletingSection?.title}&rdquo;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeletingSection(null)}
              disabled={deleteSection.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteSection.isPending}
            >
              {deleteSection.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
