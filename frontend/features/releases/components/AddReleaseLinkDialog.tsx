'use client'

import { useState } from 'react'
import { ExternalLink, Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { StatusBanner } from '@/components/shared'
// validateUrlField is not re-exported from the contributions barrel; import it
// from the types module directly (the canonical home shared with edit forms).
import { validateUrlField } from '@/features/contributions/types'
import { useAddReleaseLink } from '../hooks/useAdminReleases'
import { EXTERNAL_LINK_PLATFORMS } from '../types'

interface AddReleaseLinkDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  releaseId: number
  /**
   * Release title, quoted in the dialog description so the curator knows which
   * release they're attaching a link to.
   */
  releaseTitle: string
}

/**
 * User-facing dialog for adding an external (Listen / Buy) link to a release
 * (PSY-660). Mirrors {@link ReportEntityDialog}'s shell: `sm:max-w-md`,
 * DialogHeader (title + description), a platform Select + URL Input body, and a
 * Cancel / Add footer.
 *
 * Authorization is enforced backend-side (admin + trusted_contributor +
 * local_ambassador on POST /releases/{id}/links). The caller is responsible
 * for only rendering the trigger to authorized viewers; this dialog surfaces
 * whatever error the backend returns (e.g. a 403) via the destructive banner.
 */
export function AddReleaseLinkDialog({
  open,
  onOpenChange,
  releaseId,
  releaseTitle,
}: AddReleaseLinkDialogProps) {
  const [platform, setPlatform] = useState<string>(
    EXTERNAL_LINK_PLATFORMS[0].value
  )
  const [url, setUrl] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const addLink = useAddReleaseLink()

  // Client-side URL validation is UX-only; the backend remains the source of
  // truth. Empty is invalid here (the field is required to add a link), unlike
  // validateUrlField's "empty = clear-the-field" intent on edit forms.
  const trimmedUrl = url.trim()
  const urlFormatError = validateUrlField(url)
  const canSubmit =
    trimmedUrl.length > 0 && urlFormatError === null && !addLink.isPending

  const handleSubmit = () => {
    if (!canSubmit) return
    addLink.mutate(
      { releaseId, platform, url: trimmedUrl },
      {
        onSuccess: () => {
          setSubmitted(true)
        },
      }
    )
  }

  const handleClose = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset state when closing so the next open starts fresh.
      setPlatform(EXTERNAL_LINK_PLATFORMS[0].value)
      setUrl('')
      setSubmitted(false)
      addLink.reset()
    }
    onOpenChange(newOpen)
  }

  const showSuccess = submitted && addLink.isSuccess

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ExternalLink className="h-5 w-5 text-primary" />
            Add link
          </DialogTitle>
          <DialogDescription>
            Add a Listen / Buy link to &quot;{releaseTitle}&quot;.
          </DialogDescription>
        </DialogHeader>

        {/* Success state */}
        {showSuccess && (
          <StatusBanner variant="success">
            <div>
              <span className="font-medium text-success-foreground">
                Link added
              </span>
              <p className="mt-1 text-sm text-muted-foreground">
                Your link now appears in the release&apos;s Listen / Buy
                section.
              </p>
            </div>
          </StatusBanner>
        )}

        {/* Error state — mirrors ReportEntityDialog's destructive block.
            StatusBanner has no error variant (success / pending only). */}
        {addLink.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {(addLink.error as Error)?.message ||
              'Failed to add link. Please try again.'}
          </div>
        )}

        {/* Form */}
        {!showSuccess && (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="add-link-platform">Platform</Label>
              <Select value={platform} onValueChange={setPlatform}>
                <SelectTrigger
                  id="add-link-platform"
                  className="w-full"
                  aria-label="Link platform"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EXTERNAL_LINK_PLATFORMS.map((p) => (
                    <SelectItem key={p.value} value={p.value}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="add-link-url">URL</Label>
              <Input
                id="add-link-url"
                placeholder="https://..."
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    handleSubmit()
                  }
                }}
                aria-invalid={urlFormatError !== null}
              />
              {urlFormatError && (
                <p className="text-sm text-destructive">{urlFormatError}</p>
              )}
            </div>
          </div>
        )}

        {!showSuccess && (
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => handleClose(false)}
              disabled={addLink.isPending}
            >
              Cancel
            </Button>
            <Button onClick={handleSubmit} disabled={!canSubmit}>
              {addLink.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Adding...
                </>
              ) : (
                <>
                  <ExternalLink className="h-4 w-4 mr-2" />
                  Add link
                </>
              )}
            </Button>
          </DialogFooter>
        )}

        {showSuccess && (
          <DialogFooter>
            <Button onClick={() => handleClose(false)}>Close</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}
