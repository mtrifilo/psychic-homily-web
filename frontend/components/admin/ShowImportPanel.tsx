'use client'

import { useState, useCallback } from 'react'
import { Loader2, CheckCircle, ArrowLeft, Upload as UploadIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ImportDropZone } from './ImportDropZone'
import { ImportPreview } from './ImportPreview'
import {
  useShowImportPreview,
  useShowImportConfirm,
  type ImportPreviewResponse,
} from '@/lib/hooks/useShowImport'

type ImportState = 'idle' | 'parsing' | 'preview' | 'success'

/**
 * Main panel for importing shows from markdown files
 * State flow: idle -> parsing -> preview -> success
 */
export function ShowImportPanel() {
  const [state, setState] = useState<ImportState>('idle')
  const [fileContent, setFileContent] = useState<string | null>(null)
  const [filename, setFilename] = useState<string | null>(null)
  const [preview, setPreview] = useState<ImportPreviewResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [importedShowId, setImportedShowId] = useState<number | null>(null)
  const [isImporting, setIsImporting] = useState(false)

  const previewMutation = useShowImportPreview()
  const confirmMutation = useShowImportConfirm()

  const handleFileSelect = useCallback(
    async (content: string, name: string) => {
      setFileContent(content)
      setFilename(name)
      setError(null)
      setState('parsing')

      try {
        const result = await previewMutation.mutateAsync(content)
        setPreview(result)
        setState('preview')
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to parse markdown file'
        )
        setState('idle')
      }
    },
    [previewMutation]
  )

  const handleConfirmImport = useCallback(async () => {
    if (!fileContent) return

    setIsImporting(true)
    setError(null)

    try {
      const result = await confirmMutation.mutateAsync(fileContent)
      setImportedShowId(result.id)
      setState('success')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to import show')
    } finally {
      setIsImporting(false)
    }
  }, [fileContent, confirmMutation])

  const handleReset = useCallback(() => {
    setState('idle')
    setFileContent(null)
    setFilename(null)
    setPreview(null)
    setError(null)
    setImportedShowId(null)
  }, [])

  // Success state
  if (state === 'success') {
    return (
      <div className="space-y-6">
        <div className="rounded-lg border border-green-500/50 bg-green-500/10 p-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-green-500/20">
            <CheckCircle className="h-6 w-6 text-green-600" />
          </div>
          <h3 className="text-lg font-semibold text-green-700 dark:text-green-400">
            Show Imported Successfully
          </h3>
          <p className="mt-2 text-sm text-muted-foreground">
            The show has been created with ID #{importedShowId}
          </p>
        </div>
        <div className="flex justify-center gap-4">
          <Button variant="outline" onClick={handleReset}>
            <UploadIcon className="mr-2 h-4 w-4" />
            Import Another Show
          </Button>
        </div>
      </div>
    )
  }

  // Preview state
  if (state === 'preview' && preview) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <Button variant="ghost" size="sm" onClick={handleReset}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back
          </Button>
          <span className="text-sm text-muted-foreground">
            Previewing: {filename}
          </span>
        </div>

        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
            {error}
          </div>
        )}

        <ImportPreview preview={preview} />

        <div className="flex justify-end gap-4">
          <Button variant="outline" onClick={handleReset} disabled={isImporting}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirmImport}
            disabled={!preview.can_import || isImporting}
          >
            {isImporting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Importing...
              </>
            ) : (
              'Confirm Import'
            )}
          </Button>
        </div>
      </div>
    )
  }

  // Idle/parsing state
  return (
    <div className="space-y-6">
      <div className="text-center">
        <h3 className="text-lg font-semibold">Import Show from Markdown</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Upload a markdown file exported from the show export feature
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
          {error}
        </div>
      )}

      <ImportDropZone
        onFileSelect={handleFileSelect}
        isLoading={state === 'parsing'}
        disabled={state === 'parsing'}
      />

      {state === 'parsing' && (
        <div className="flex items-center justify-center gap-2 text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Parsing markdown file...</span>
        </div>
      )}

      {fileContent && state === 'idle' && (
        <div className="flex justify-center">
          <Button
            onClick={() => handleFileSelect(fileContent, filename || 'file.md')}
          >
            Preview Import
          </Button>
        </div>
      )}
    </div>
  )
}
