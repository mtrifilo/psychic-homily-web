'use client'

import { useState, useCallback, DragEvent } from 'react'
import { Upload, FileText, X } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ImportDropZoneProps {
  onFileSelect: (content: string, filename: string) => void
  isLoading?: boolean
  disabled?: boolean
}

/**
 * Drag-and-drop zone for importing markdown files
 * Uses native HTML5 drag-and-drop API
 */
export function ImportDropZone({
  onFileSelect,
  isLoading = false,
  disabled = false,
}: ImportDropZoneProps) {
  const [isDragging, setIsDragging] = useState(false)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)

  const handleDragEnter = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(true)
  }, [])

  const handleDragLeave = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
  }, [])

  const handleDragOver = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const processFile = useCallback(
    async (file: File) => {
      if (!file.name.endsWith('.md')) {
        alert('Please select a markdown (.md) file')
        return
      }

      try {
        const content = await file.text()
        setSelectedFile(file.name)
        onFileSelect(content, file.name)
      } catch (error) {
        alert('Failed to read file')
      }
    },
    [onFileSelect]
  )

  const handleDrop = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.stopPropagation()
      setIsDragging(false)

      if (disabled || isLoading) return

      const files = e.dataTransfer.files
      if (files.length > 0) {
        processFile(files[0])
      }
    },
    [disabled, isLoading, processFile]
  )

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files
      if (files && files.length > 0) {
        processFile(files[0])
      }
      // Reset input so the same file can be selected again
      e.target.value = ''
    },
    [processFile]
  )

  const clearSelection = useCallback(() => {
    setSelectedFile(null)
  }, [])

  return (
    <div
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      className={cn(
        'relative rounded-lg border-2 border-dashed p-8 transition-colors',
        isDragging && !disabled
          ? 'border-primary bg-primary/5'
          : 'border-border hover:border-primary/50',
        disabled && 'cursor-not-allowed opacity-50'
      )}
    >
      <input
        type="file"
        accept=".md"
        onChange={handleFileInput}
        disabled={disabled || isLoading}
        className="absolute inset-0 cursor-pointer opacity-0"
      />

      <div className="flex flex-col items-center justify-center text-center">
        {selectedFile ? (
          <>
            <div className="flex items-center gap-2 text-primary">
              <FileText className="h-8 w-8" />
              <span className="font-medium">{selectedFile}</span>
              <button
                onClick={e => {
                  e.stopPropagation()
                  clearSelection()
                }}
                className="ml-2 rounded-full p-1 hover:bg-muted"
                disabled={disabled}
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              Click preview to see the import details
            </p>
          </>
        ) : (
          <>
            <Upload
              className={cn(
                'h-12 w-12',
                isDragging ? 'text-primary' : 'text-muted-foreground'
              )}
            />
            <p className="mt-4 font-medium">
              {isDragging
                ? 'Drop the file here'
                : 'Drag and drop a markdown file'}
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              or click to select a file
            </p>
            <p className="mt-3 text-xs text-muted-foreground">
              Supports .md files exported from the show export feature
            </p>
          </>
        )}
      </div>
    </div>
  )
}
