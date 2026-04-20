'use client'

import { useState, useEffect, useCallback, useMemo } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Search,
  Upload,
  Inbox,
  ArrowRight,
  AlertCircle,
  CheckCircle2,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { TagOfficialIndicator } from '../components/TagOfficialIndicator'
import { getCategoryColor, getCategoryLabel } from '../types'
import type {
  BulkAliasImportItem,
  BulkAliasImportResult,
} from '../types'
import { useAllTagAliases, useBulkImportAliases } from './useAdminTags'

const PAGE_SIZE = 50

// parseCSV turns a pasted/uploaded blob into `{alias, canonical}` rows.
// Accepts either comma- or tab-separated values, tolerates a header row if
// either column literally matches "alias"/"canonical", and skips blank lines
// and `#` comments so admins can annotate their source files.
export function parseCSV(raw: string): BulkAliasImportItem[] {
  const rows: BulkAliasImportItem[] = []
  const lines = raw.split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue
    if (line.startsWith('#')) continue

    const parts = line.includes('\t')
      ? line.split('\t')
      : line.split(',')
    if (parts.length < 2) continue

    const alias = parts[0].trim()
    const canonical = parts[1].trim()
    if (!alias && !canonical) continue

    // Skip header row if present
    if (
      i === 0 &&
      alias.toLowerCase() === 'alias' &&
      canonical.toLowerCase() === 'canonical'
    ) {
      continue
    }

    rows.push({ alias, canonical })
  }
  return rows
}

export function AliasListing() {
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [page, setPage] = useState(0)
  const [importOpen, setImportOpen] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => {
      setDebouncedSearch(searchInput)
      setPage(0)
    }, 300)
    return () => clearTimeout(t)
  }, [searchInput])

  const { data, isLoading, error } = useAllTagAliases({
    search: debouncedSearch || undefined,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  const aliases = data?.aliases ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="text-lg font-semibold">Global alias listing</h3>
          <p className="text-sm text-muted-foreground">
            Every alias across every tag. Search either the alias text or the
            canonical tag name.
          </p>
        </div>
        <Button onClick={() => setImportOpen(true)}>
          <Upload className="mr-2 h-4 w-4" />
          Bulk import
        </Button>
      </div>

      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search aliases or canonical tag name..."
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="pl-9"
        />
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
          <p className="text-destructive">
            {error instanceof Error ? error.message : 'Failed to load aliases.'}
          </p>
        </div>
      )}

      {!isLoading && !error && aliases.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">No aliases found</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            {debouncedSearch
              ? 'No aliases match your search.'
              : 'No aliases yet. Add aliases from each tag, or bulk import a CSV.'}
          </p>
        </div>
      )}

      {!isLoading && !error && aliases.length > 0 && (
        <>
          <div className="text-sm text-muted-foreground">
            {total} alias{total !== 1 ? 'es' : ''}
            {debouncedSearch && ` matching "${debouncedSearch}"`}
          </div>

          <div className="rounded-lg border">
            {aliases.map((a, idx) => (
              <div
                key={a.id}
                className={`flex items-center gap-3 p-3 ${idx > 0 ? 'border-t' : ''}`}
              >
                <div className="flex-1 min-w-0 flex items-center gap-3 flex-wrap">
                  <span className="font-mono text-sm">{a.alias}</span>
                  <ArrowRight className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                  <Link
                    href={`/tags/${a.tag_slug}`}
                    className="font-medium text-sm hover:underline"
                  >
                    {a.tag_name}
                  </Link>
                  <Badge
                    variant="outline"
                    className={`text-xs ${getCategoryColor(a.tag_category)}`}
                  >
                    {getCategoryLabel(a.tag_category)}
                  </Badge>
                  {a.tag_is_official && (
                    <TagOfficialIndicator size="sm" tagName={a.tag_name} />
                  )}
                </div>
              </div>
            ))}
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={page === 0}
              >
                Previous
              </Button>
              <span className="text-sm text-muted-foreground">
                Page {page + 1} of {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                disabled={page >= totalPages - 1}
              >
                Next
              </Button>
            </div>
          )}
        </>
      )}

      <BulkImportDialog
        open={importOpen}
        onOpenChange={setImportOpen}
      />
    </div>
  )
}

// ============================================================================
// Bulk Import Dialog
// ============================================================================

function BulkImportDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const [text, setText] = useState('')
  const [parseError, setParseError] = useState<string | null>(null)
  const [result, setResult] = useState<BulkAliasImportResult | null>(null)
  const bulkImport = useBulkImportAliases()

  const parsedRows = useMemo(() => {
    if (!text.trim()) return []
    try {
      return parseCSV(text)
    } catch {
      return []
    }
  }, [text])

  const handleFileUpload = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (!file) return
      const reader = new FileReader()
      reader.onload = () => {
        setText(String(reader.result ?? ''))
        setParseError(null)
        setResult(null)
      }
      reader.onerror = () => {
        setParseError('Failed to read file')
      }
      reader.readAsText(file)
    },
    []
  )

  const handleSubmit = useCallback(() => {
    setParseError(null)
    setResult(null)
    if (parsedRows.length === 0) {
      setParseError('Add at least one alias,canonical row.')
      return
    }
    bulkImport.mutate(parsedRows, {
      onSuccess: (data) => setResult(data),
      onError: (err) => {
        setParseError(err instanceof Error ? err.message : 'Import failed.')
      },
    })
  }, [parsedRows, bulkImport])

  const handleClose = useCallback(() => {
    setText('')
    setParseError(null)
    setResult(null)
    onOpenChange(false)
  }, [onOpenChange])

  return (
    <Dialog open={open} onOpenChange={(v) => (v ? onOpenChange(v) : handleClose())}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Bulk import aliases</DialogTitle>
          <DialogDescription>
            Upload a CSV or paste <code className="font-mono">alias,canonical</code> lines.
            Canonical can be a tag slug or exact tag name. Comments (<code>#</code>) and blank lines are ignored.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="alias-csv-file">Upload CSV file</Label>
            <Input
              id="alias-csv-file"
              type="file"
              accept=".csv,.txt,text/csv,text/plain"
              onChange={handleFileUpload}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="alias-csv-text">Or paste rows</Label>
            <Textarea
              id="alias-csv-text"
              value={text}
              onChange={(e) => {
                setText(e.target.value)
                setResult(null)
              }}
              placeholder={`alias,canonical\ndnb,drum-and-bass\nhiphop,hip-hop`}
              rows={8}
              className="font-mono text-xs"
            />
            <p className="text-xs text-muted-foreground">
              Parsed {parsedRows.length} row{parsedRows.length === 1 ? '' : 's'}.
            </p>
          </div>

          {parseError && (
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive flex items-start gap-2">
              <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" />
              <span>{parseError}</span>
            </div>
          )}

          {result && (
            <div className="space-y-3">
              <div className="rounded-lg border border-green-500/30 bg-green-500/10 p-3 text-sm text-green-400 flex items-start gap-2">
                <CheckCircle2 className="h-4 w-4 mt-0.5 flex-shrink-0" />
                <span>
                  Imported {result.imported} alias{result.imported === 1 ? '' : 'es'}.
                  {result.skipped.length > 0 &&
                    ` Skipped ${result.skipped.length} row${result.skipped.length === 1 ? '' : 's'}.`}
                </span>
              </div>

              {result.skipped.length > 0 && (
                <div className="rounded-lg border border-amber-500/30">
                  <div className="bg-amber-500/10 px-3 py-2 text-sm font-medium text-amber-400 flex items-center gap-2 border-b border-amber-500/30">
                    <X className="h-4 w-4" />
                    Rejected rows
                  </div>
                  <div className="max-h-48 overflow-y-auto">
                    {result.skipped.map((s) => (
                      <div
                        key={`${s.row}-${s.alias}`}
                        className="flex items-start gap-3 px-3 py-2 text-xs border-b last:border-b-0"
                      >
                        <span className="font-mono text-muted-foreground flex-shrink-0">
                          row {s.row}
                        </span>
                        <div className="flex-1 min-w-0">
                          <div className="font-mono">
                            {s.alias || '(empty)'} → {s.canonical || '(empty)'}
                          </div>
                          <div className="text-amber-400 mt-0.5">{s.reason}</div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={bulkImport.isPending}>
            {result ? 'Close' : 'Cancel'}
          </Button>
          {!result && (
            <Button
              onClick={handleSubmit}
              disabled={bulkImport.isPending || parsedRows.length === 0}
            >
              {bulkImport.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Importing...
                </>
              ) : (
                `Import ${parsedRows.length} row${parsedRows.length === 1 ? '' : 's'}`
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export default AliasListing
