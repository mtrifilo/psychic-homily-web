'use client'

import { useState } from 'react'
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
}: FieldNoteFormProps) {
  const [body, setBody] = useState('')
  const [soundQuality, setSoundQuality] = useState(0)
  const [crowdEnergy, setCrowdEnergy] = useState(0)
  const [notableMoments, setNotableMoments] = useState('')
  const [setlistSpoiler, setSetlistSpoiler] = useState(false)
  const [showArtistId, setShowArtistId] = useState<number | undefined>(undefined)
  const [songPosition, setSongPosition] = useState('')

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

    onSubmit(input)
    // Reset form
    setBody('')
    setSoundQuality(0)
    setCrowdEnergy(0)
    setNotableMoments('')
    setSetlistSpoiler(false)
    setShowArtistId(undefined)
    setSongPosition('')
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
          Post Field Note
        </Button>
      </div>
    </form>
  )
}
