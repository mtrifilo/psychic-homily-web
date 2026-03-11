'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
import {
  Plus,
  Pencil,
  Trash2,
  Loader2,
  AlertCircle,
  GripVertical,
} from 'lucide-react'
import {
  useOwnSections,
  useCreateSection,
  useUpdateSection,
  useDeleteSection,
} from '@/lib/hooks/useContributorProfile'
import type { ProfileSectionResponse } from '@/lib/types/contributor'

const MAX_SECTIONS = 3

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
  const canAddMore = sections.length < MAX_SECTIONS

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
        <CardContent className="p-6 flex justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium">Custom Sections</h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            Add up to {MAX_SECTIONS} custom sections to your profile ({sections.length}/{MAX_SECTIONS})
          </p>
        </div>
        {canAddMore && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setNewTitle('')
              setNewContent('')
              setFormError(null)
              setCreateDialogOpen(true)
            }}
            className="gap-1.5"
          >
            <Plus className="h-4 w-4" />
            Add Section
          </Button>
        )}
      </div>

      {sections.length === 0 ? (
        <Card className="bg-muted/30 border-border/50 border-dashed">
          <CardContent className="p-6 text-center">
            <p className="text-sm text-muted-foreground">
              No custom sections yet. Add sections to personalize your profile.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {sections
            .sort((a, b) => a.position - b.position)
            .map(section => (
              <Card key={section.id} className="bg-muted/30 border-border/50">
                <CardContent className="p-4">
                  <div className="flex items-start gap-3">
                    <GripVertical className="h-5 w-5 text-muted-foreground/40 mt-0.5 shrink-0" />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <h4 className="text-sm font-medium">{section.title}</h4>
                        {!section.is_visible && (
                          <span className="text-xs text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
                            Hidden
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground line-clamp-2">
                        {section.content}
                      </p>
                    </div>
                    <div className="flex items-center gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => openEditDialog(section)}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => setDeletingSection(section)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
        </div>
      )}

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
              <p className="text-xs text-muted-foreground text-right">
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
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
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
              <p className="text-xs text-muted-foreground text-right">
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
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
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
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              )}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
