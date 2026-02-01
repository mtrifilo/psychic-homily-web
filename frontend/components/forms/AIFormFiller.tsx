'use client'

import { useState, useCallback, useRef } from 'react'
import {
  Sparkles,
  ChevronDown,
  ChevronUp,
  X,
  Loader2,
  CheckCircle2,
  AlertCircle,
  ImageIcon,
} from 'lucide-react'
import { useShowExtraction } from '@/lib/hooks/useShowExtraction'
import type { ExtractedShowData } from '@/lib/types/extraction'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'

const MAX_IMAGE_SIZE = 10 * 1024 * 1024 // 10MB
const SUPPORTED_IMAGE_TYPES = [
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/webp',
]

interface AIFormFillerProps {
  /** Callback when extraction is successful */
  onExtracted: (data: ExtractedShowData) => void
}

export function AIFormFiller({ onExtracted }: AIFormFillerProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  const [textInput, setTextInput] = useState('')
  const [imageFile, setImageFile] = useState<File | null>(null)
  const [imagePreview, setImagePreview] = useState<string | null>(null)
  const [extractionResult, setExtractionResult] =
    useState<ExtractedShowData | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const fileInputRef = useRef<HTMLInputElement>(null)

  const { mutate, isPending, error, reset } = useShowExtraction()

  const handleTextChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setTextInput(e.target.value)
    // Reset previous results when input changes
    setExtractionResult(null)
    setWarnings([])
    reset()
  }

  const handleImageSelect = useCallback(
    (file: File) => {
      if (!SUPPORTED_IMAGE_TYPES.includes(file.type)) {
        alert(
          `Unsupported image type. Please use: ${SUPPORTED_IMAGE_TYPES.join(', ')}`
        )
        return
      }

      if (file.size > MAX_IMAGE_SIZE) {
        alert('Image is too large. Maximum size is 10MB.')
        return
      }

      setImageFile(file)
      setExtractionResult(null)
      setWarnings([])
      reset()

      // Create preview
      const reader = new FileReader()
      reader.onload = e => {
        setImagePreview(e.target?.result as string)
      }
      reader.readAsDataURL(file)
    },
    [reset]
  )

  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) {
      handleImageSelect(file)
    }
  }

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      e.stopPropagation()

      const file = e.dataTransfer.files[0]
      if (file) {
        handleImageSelect(file)
      }
    },
    [handleImageSelect]
  )

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }

  const clearImage = () => {
    setImageFile(null)
    setImagePreview(null)
    setExtractionResult(null)
    setWarnings([])
    reset()
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  const handleExtract = async () => {
    const hasText = textInput.trim().length > 0
    const hasImage = imageFile !== null && imagePreview !== null

    if (!hasText && !hasImage) return

    if (hasImage) {
      // Extract base64 data from data URL
      const base64Data = imagePreview!.split(',')[1]
      const mediaType = imageFile!.type as
        | 'image/jpeg'
        | 'image/png'
        | 'image/gif'
        | 'image/webp'

      // Use 'both' if we have text context, otherwise just 'image'
      mutate(
        {
          type: hasText ? 'both' : 'image',
          image_data: base64Data,
          media_type: mediaType,
          text: hasText ? textInput : undefined,
        },
        {
          onSuccess: response => {
            if (response.data) {
              setExtractionResult(response.data)
              setWarnings(response.warnings || [])
              onExtracted(response.data)
            }
          },
        }
      )
    } else {
      // Text only
      mutate(
        { type: 'text', text: textInput },
        {
          onSuccess: response => {
            if (response.data) {
              setExtractionResult(response.data)
              setWarnings(response.warnings || [])
              onExtracted(response.data)
            }
          },
        }
      )
    }
  }

  const canExtract = textInput.trim().length > 0 || imageFile !== null

  return (
    <Card className="border-border/50 bg-card/50 backdrop-blur-sm mb-4 py-0">
      <CardHeader
        className="cursor-pointer py-3 flex flex-row items-center justify-between"
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <CardTitle className="flex items-center gap-2 text-base">
          <Sparkles className="h-4 w-4 text-primary" />
          AI Form Filler-Outer
        </CardTitle>
        {isExpanded ? (
          <ChevronUp className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        )}
      </CardHeader>

      {isExpanded && (
        <CardContent className="pt-0 pb-4 space-y-4">
          <p className="text-sm text-muted-foreground">
            Upload a flyer image and/or paste show details to automatically fill
            the form.
          </p>

          {/* Image Drop Zone */}
          {imagePreview ? (
            <div className="relative">
              <img
                src={imagePreview}
                alt="Uploaded flyer"
                className="max-h-48 rounded-md border border-input object-contain mx-auto"
              />
              <Button
                variant="ghost"
                size="icon"
                className="absolute top-2 right-2 h-7 w-7 bg-background/80 hover:bg-background"
                onClick={clearImage}
                disabled={isPending}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          ) : (
            <div
              className="border-2 border-dashed border-input rounded-md p-4 text-center hover:border-primary/50 transition-colors cursor-pointer"
              onDrop={handleDrop}
              onDragOver={handleDragOver}
              onClick={() => fileInputRef.current?.click()}
            >
              <div className="flex flex-col sm:flex-row items-center justify-center gap-2 sm:gap-3">
                <div className="flex items-center justify-center h-10 w-10 rounded-full bg-muted">
                  <ImageIcon className="h-5 w-5 text-muted-foreground" />
                </div>
                <div className="text-center sm:text-left">
                  <p className="text-sm text-muted-foreground">
                    <span className="hidden sm:inline">Drop a flyer image here, or </span>
                    <span className="sm:hidden">Tap to </span>
                    <span className="text-primary">
                      <span className="hidden sm:inline">click to select</span>
                      <span className="sm:hidden">upload a flyer image</span>
                    </span>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    JPEG, PNG, GIF, WebP (max 10MB)
                  </p>
                </div>
              </div>
              <input
                ref={fileInputRef}
                type="file"
                accept={SUPPORTED_IMAGE_TYPES.join(',')}
                className="hidden"
                onChange={handleFileInputChange}
                disabled={isPending}
              />
            </div>
          )}

          {/* Text Input */}
          <textarea
            className="w-full min-h-[180px] rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 resize-y"
            placeholder={
              imagePreview
                ? 'Add any details the image might be missing (venue, time, price, etc.)...'
                : 'Paste show details, flyer text, or event info...\n\nExample:\nThe National with Bartees Strange\nValley Bar, Phoenix AZ\nFriday, February 15\nDoors 7pm / Show 8pm\n$35 / 21+'
            }
            value={textInput}
            onChange={handleTextChange}
            disabled={isPending}
          />

          {/* Extract Button */}
          <Button
            onClick={handleExtract}
            disabled={!canExtract || isPending}
            className="w-full"
          >
            {isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                Extracting...
              </>
            ) : (
              <>
                <Sparkles className="h-4 w-4 mr-2" />
                Extract Show Info
              </>
            )}
          </Button>

          {/* Error Display */}
          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          )}

          {/* Success Display */}
          {extractionResult && (
            <div className="rounded-md bg-primary/5 border border-primary/20 p-3 space-y-2">
              <div className="flex items-center gap-2 text-sm font-medium text-primary">
                <CheckCircle2 className="h-4 w-4" />
                Extraction Complete
              </div>
              <div className="text-sm text-muted-foreground space-y-1">
                {/* Artists */}
                {extractionResult.artists.length > 0 && (
                  <div className="flex items-center gap-2 flex-wrap">
                    <span>Artists:</span>
                    {extractionResult.artists.map((artist, i) => (
                      <Badge
                        key={i}
                        variant={artist.matched_id ? 'default' : 'secondary'}
                        className="text-xs"
                      >
                        {artist.matched_name || artist.name}
                        {artist.matched_id && (
                          <CheckCircle2 className="h-3 w-3 ml-1" />
                        )}
                      </Badge>
                    ))}
                  </div>
                )}

                {/* Venue */}
                {extractionResult.venue && (
                  <div className="flex items-center gap-2">
                    <span>Venue:</span>
                    <Badge
                      variant={
                        extractionResult.venue.matched_id
                          ? 'default'
                          : 'secondary'
                      }
                      className="text-xs"
                    >
                      {extractionResult.venue.matched_name ||
                        extractionResult.venue.name}
                      {extractionResult.venue.matched_id && (
                        <CheckCircle2 className="h-3 w-3 ml-1" />
                      )}
                    </Badge>
                  </div>
                )}

                {/* Date/Time */}
                {(extractionResult.date || extractionResult.time) && (
                  <div>
                    Date:{' '}
                    {extractionResult.date && (
                      <span className="font-medium">
                        {new Date(
                          extractionResult.date + 'T12:00:00'
                        ).toLocaleDateString('en-US', {
                          month: 'short',
                          day: 'numeric',
                          year: 'numeric',
                        })}
                      </span>
                    )}
                    {extractionResult.time && (
                      <span className="font-medium">
                        {' '}
                        at{' '}
                        {new Date(
                          `2000-01-01T${extractionResult.time}`
                        ).toLocaleTimeString('en-US', {
                          hour: 'numeric',
                          minute: '2-digit',
                        })}
                      </span>
                    )}
                  </div>
                )}

                {/* Cost/Ages */}
                {(extractionResult.cost || extractionResult.ages) && (
                  <div>
                    {extractionResult.cost && (
                      <span className="font-medium">
                        {extractionResult.cost}
                      </span>
                    )}
                    {extractionResult.cost && extractionResult.ages && ' / '}
                    {extractionResult.ages && (
                      <span className="font-medium">
                        {extractionResult.ages}
                      </span>
                    )}
                  </div>
                )}
              </div>

              {/* Warnings */}
              {warnings.length > 0 && (
                <div className="text-xs text-amber-600 dark:text-amber-400 mt-2">
                  {warnings.map((warning, i) => (
                    <div key={i}>{warning}</div>
                  ))}
                </div>
              )}
            </div>
          )}
        </CardContent>
      )}
    </Card>
  )
}
