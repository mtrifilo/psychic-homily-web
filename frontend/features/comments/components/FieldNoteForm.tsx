'use client'

import { useEffect, useState } from 'react'
import { Loader2, Star } from 'lucide-react'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import type { CreateFieldNoteInput } from '../types'

interface ShowArtist {
  id: number
  name: string
}

interface FieldNoteFormProps {
  onSubmit: (input: CreateFieldNoteInput) => void
  artists?: ShowArtist[]
  isPending?: boolean
  disabled?: boolean
  disabledMessage?: string
  /**
   * PSY-608: optional inline error banner. When set, renders a
   * destructive-styled message above the textarea. Mirrors CommentForm's
   * errorMessage; reuse the same `formatCommentSubmissionError` helper for
   * 429 countdown copy.
   */
  errorMessage?: string | null
  /**
   * PSY-608: bumping this number signals "submission succeeded — clear
   * the form." Mirrors CommentForm.resetSignal. Without this, the previous
   * eager-clear-on-submit behaviour discarded the draft on 4xx errors.
   */
  resetSignal?: number
  /**
   * PSY-568: pre-checks the "I attended this show" checkbox when the
   * current user has Going set on this show. The checkbox value sent on
   * submit is authoritative — the user can uncheck it (e.g. they were
   * marked Going but couldn't actually make it). Snapshot semantics:
   * toggling Going after posting does NOT flip the badge on existing
   * notes.
   */
  defaultVerifiedAttendee?: boolean
  /**
   * PSY-567: optional initial values for edit mode. When supplied, all
   * fields pre-populate from the existing note. resetSignal still works
   * but resets back to these initial values (not blank). Mode visibility
   * stays identical to create — the same structured-data fields are
   * shown, satisfying the "edit as a unit, not body-only" AC.
   */
  initialValues?: {
    body: string
    sound_quality?: number | null
    crowd_energy?: number | null
    notable_moments?: string | null
    setlist_spoiler?: boolean
    show_artist_id?: number | null
    song_position?: number | null
    verified_attendee?: boolean
  }
  /**
   * PSY-567: submit-button label override. Defaults to "Post Field Note"
   * for the create path; edit mode uses "Save".
   */
  submitLabel?: string
  /**
   * PSY-567: optional Cancel callback. When supplied, a secondary
   * "Cancel" button renders alongside Submit so the user can abort an
   * in-progress edit without losing the rendered note.
   */
  onCancel?: () => void
}

function StarRating({
  value,
  onChange,
  label,
  testId,
}: {
  value: number
  onChange: (v: number) => void
  label: string
  testId: string
}) {
  return (
    <div className="flex items-center gap-2">
      <Label className="text-sm text-muted-foreground min-w-[100px]">{label}</Label>
      <div className="flex items-center gap-0.5" data-testid={testId}>
        {[1, 2, 3, 4, 5].map((star) => (
          <button
            key={star}
            type="button"
            onClick={() => onChange(value === star ? 0 : star)}
            className="p-0.5 hover:scale-110 transition-transform"
            aria-label={`${star} star${star !== 1 ? 's' : ''}`}
          >
            <Star
              className={`h-5 w-5 ${
                star <= value
                  ? 'fill-yellow-400 text-yellow-400'
                  : 'text-muted-foreground/40'
              }`}
            />
          </button>
        ))}
        {value > 0 && (
          <span className="text-xs text-muted-foreground ml-1">{value}/5</span>
        )}
      </div>
    </div>
  )
}

export function FieldNoteForm({
  onSubmit,
  artists = [],
  isPending = false,
  disabled = false,
  disabledMessage,
  errorMessage,
  resetSignal,
  defaultVerifiedAttendee = false,
  initialValues,
  submitLabel,
  onCancel,
}: FieldNoteFormProps) {
  // PSY-567: in edit mode (initialValues supplied) hydrate from the
  // existing note. Otherwise default to empty/zero (create path, unchanged).
  const [body, setBody] = useState(initialValues?.body ?? '')
  const [soundQuality, setSoundQuality] = useState(initialValues?.sound_quality ?? 0)
  const [crowdEnergy, setCrowdEnergy] = useState(initialValues?.crowd_energy ?? 0)
  const [notableMoments, setNotableMoments] = useState(initialValues?.notable_moments ?? '')
  const [setlistSpoiler, setSetlistSpoiler] = useState(
    initialValues?.setlist_spoiler ?? false
  )
  const [showArtistId, setShowArtistId] = useState<number | undefined>(
    initialValues?.show_artist_id ?? undefined
  )
  const [songPosition, setSongPosition] = useState(
    initialValues?.song_position != null ? String(initialValues.song_position) : ''
  )
  // PSY-568: self-claim "I attended this show". Initial state mirrors the
  // user's current Going RSVP; falls back to false. Re-syncs when the
  // default changes (e.g. attendance loads after the form mounts).
  // PSY-567: in edit mode, the stored value takes precedence over the
  // Going-RSVP default — the user's prior self-claim is what they're
  // editing.
  const [verifiedAttendee, setVerifiedAttendee] = useState(
    initialValues?.verified_attendee ?? defaultVerifiedAttendee
  )
  useEffect(() => {
    // PSY-567: only re-sync from Going status on the CREATE path. Editing
    // an existing note must not silently overwrite the prior self-claim
    // when attendance data loads after the form mounts.
    if (initialValues) return
    setVerifiedAttendee(defaultVerifiedAttendee)
  }, [defaultVerifiedAttendee, initialValues])

  // PSY-608: parent bumps resetSignal from mutation onSuccess. Mirrors the
  // CommentForm pattern so a 4xx response keeps the user's draft intact.
  // PSY-568: also resets the verified-attendee checkbox to the current
  // default. defaultVerifiedAttendee is intentionally OMITTED from the
  // dep array — its own useEffect above handles re-sync when Going status
  // changes, and including it here would wipe the user's draft on every
  // attendance refetch.
  // PSY-567: in edit mode, reset returns to initialValues (not blank) so
  // a save-then-reopen cycle shows the freshly-saved state.
  useEffect(() => {
    if (resetSignal === undefined) return
    if (initialValues) {
      setBody(initialValues.body)
      setSoundQuality(initialValues.sound_quality ?? 0)
      setCrowdEnergy(initialValues.crowd_energy ?? 0)
      setNotableMoments(initialValues.notable_moments ?? '')
      setSetlistSpoiler(initialValues.setlist_spoiler ?? false)
      setShowArtistId(initialValues.show_artist_id ?? undefined)
      setSongPosition(
        initialValues.song_position != null
          ? String(initialValues.song_position)
          : ''
      )
      setVerifiedAttendee(initialValues.verified_attendee ?? false)
      return
    }
    setBody('')
    setSoundQuality(0)
    setCrowdEnergy(0)
    setNotableMoments('')
    setSetlistSpoiler(false)
    setShowArtistId(undefined)
    setSongPosition('')
    setVerifiedAttendee(defaultVerifiedAttendee)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetSignal])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = body.trim()
    if (!trimmed) return

    const input: CreateFieldNoteInput = { body: trimmed }

    if (soundQuality > 0) input.sound_quality = soundQuality
    if (crowdEnergy > 0) input.crowd_energy = crowdEnergy
    if (notableMoments.trim()) input.notable_moments = notableMoments.trim()
    if (setlistSpoiler) input.setlist_spoiler = true
    if (showArtistId) input.show_artist_id = showArtistId
    if (songPosition && parseInt(songPosition, 10) > 0) {
      input.song_position = parseInt(songPosition, 10)
    }
    // PSY-568: always send the flag so the backend stores the explicit
    // user choice (default-false on the server matches an unchecked box).
    input.verified_attendee = verifiedAttendee

    onSubmit(input)
    // PSY-608: reset is parent-driven via resetSignal (mirrors CommentForm).
    // Eagerly clearing here previously discarded the draft when the request
    // came back 4xx; now the parent bumps resetSignal only on success.
  }

  if (disabled && disabledMessage) {
    return (
      <div
        className="rounded-lg border border-border bg-muted/30 p-4 text-sm text-muted-foreground text-center"
        data-testid="field-note-form-disabled"
      >
        {disabledMessage}
      </div>
    )
  }

  const isSubmitDisabled = !body.trim() || isPending

  return (
    <form onSubmit={handleSubmit} className="space-y-4" data-testid="field-note-form">
      {/* PSY-608: inline error banner — same shape as CommentForm. Parent
          wires this from the createFieldNote mutation error. */}
      {errorMessage && (
        <div
          className="rounded-md border border-red-800 bg-red-950/50 p-3"
          role="alert"
          data-testid="field-note-form-error"
        >
          <p className="text-sm text-red-400">{errorMessage}</p>
        </div>
      )}
      {/* Body */}
      <Textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Share your experience at this show..."
        rows={3}
        disabled={isPending}
        data-testid="field-note-textarea"
      />

      {/* Structured fields */}
      <div className="space-y-3 rounded-lg border border-border/50 bg-muted/20 p-3">
        <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          Optional Details
        </p>

        {/* PSY-568: self-claim "I attended this show". Pre-checked when the
            user has Going set; user can override (snapshot at post time). */}
        <div className="flex items-center gap-2">
          <Checkbox
            id="verified-attendee"
            checked={verifiedAttendee}
            onCheckedChange={(checked) => setVerifiedAttendee(checked === true)}
            disabled={isPending}
            data-testid="verified-attendee-checkbox"
          />
          <Label
            htmlFor="verified-attendee"
            className="text-sm text-foreground cursor-pointer"
          >
            I attended this show
          </Label>
        </div>

        {/* Star ratings */}
        <StarRating
          value={soundQuality}
          onChange={setSoundQuality}
          label="Sound Quality"
          testId="sound-quality-rating"
        />
        <StarRating
          value={crowdEnergy}
          onChange={setCrowdEnergy}
          label="Crowd Energy"
          testId="crowd-energy-rating"
        />

        {/* Notable moments */}
        <div className="flex items-start gap-2">
          <Label className="text-sm text-muted-foreground min-w-[100px] mt-2">
            Notable Moments
          </Label>
          <Input
            value={notableMoments}
            onChange={(e) => setNotableMoments(e.target.value)}
            placeholder="e.g. Played 3 new songs, surprise guest"
            disabled={isPending}
            data-testid="notable-moments-input"
          />
        </div>

        {/* Artist picker */}
        {artists.length > 0 && (
          <div className="flex items-center gap-2">
            <Label className="text-sm text-muted-foreground min-w-[100px]">
              Artist Set
            </Label>
            <select
              value={showArtistId ?? ''}
              onChange={(e) =>
                setShowArtistId(e.target.value ? Number(e.target.value) : undefined)
              }
              className="h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] outline-none"
              disabled={isPending}
              data-testid="artist-select"
            >
              <option value="">Any / General</option>
              {artists.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Song position */}
        <div className="flex items-center gap-2">
          <Label className="text-sm text-muted-foreground min-w-[100px]">
            Song #
          </Label>
          <Input
            type="number"
            min={1}
            value={songPosition}
            onChange={(e) => setSongPosition(e.target.value)}
            placeholder="Position in setlist"
            className="w-40"
            disabled={isPending}
            data-testid="song-position-input"
          />
        </div>

        {/* Setlist spoiler */}
        <div className="flex items-center gap-2">
          <Checkbox
            id="setlist-spoiler"
            checked={setlistSpoiler}
            onCheckedChange={(checked) => setSetlistSpoiler(checked === true)}
            disabled={isPending}
            data-testid="setlist-spoiler-checkbox"
          />
          <Label htmlFor="setlist-spoiler" className="text-sm text-muted-foreground cursor-pointer">
            Contains setlist spoilers
          </Label>
        </div>
      </div>

      {/* Submit */}
      <div className="flex items-center gap-2">
        <Button
          type="submit"
          size="sm"
          disabled={isSubmitDisabled}
          data-testid="field-note-submit"
        >
          {isPending && <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />}
          {submitLabel ?? 'Post Field Note'}
        </Button>
        {onCancel && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            disabled={isPending}
            data-testid="field-note-cancel"
          >
            Cancel
          </Button>
        )}
      </div>
    </form>
  )
}
