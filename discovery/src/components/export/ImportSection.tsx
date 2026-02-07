import { useState } from 'react'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Checkbox } from '../ui/checkbox'
import { Label } from '../ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card'
import { LoadingSpinner } from '../shared/LoadingSpinner'
import { ErrorAlert } from '../shared/ErrorAlert'
import { useDataImport } from '../../lib/hooks/useImport'
import { cn } from '../../lib/utils'
import type {
  ExportedShow,
  ExportedArtist,
  ExportedVenue,
  DataImportResult,
} from '../../lib/types'

interface ImportSectionProps {
  selectedShows: ExportedShow[]
  selectedArtists: ExportedArtist[]
  selectedVenues: ExportedVenue[]
  targetEnv: string
  hasTargetToken: boolean
  isProduction: boolean
}

export function ImportSection({
  selectedShows,
  selectedArtists,
  selectedVenues,
  targetEnv,
  hasTargetToken,
  isProduction,
}: ImportSectionProps) {
  const [isDryRun, setIsDryRun] = useState(true)
  const [importResult, setImportResult] = useState<DataImportResult | null>(null)

  const { mutate: importData, isPending: isImporting, error } = useDataImport()

  const totalSelected =
    selectedShows.length + selectedArtists.length + selectedVenues.length

  const handleImport = () => {
    setImportResult(null)

    importData(
      {
        shows: selectedShows.length > 0 ? selectedShows : undefined,
        artists: selectedArtists.length > 0 ? selectedArtists : undefined,
        venues: selectedVenues.length > 0 ? selectedVenues : undefined,
        dryRun: isDryRun,
      },
      {
        onSuccess: (result) => {
          setImportResult(result)
        },
      }
    )
  }

  if (totalSelected === 0) {
    return null
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-4">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Upload to {targetEnv}</CardTitle>
              <p className="text-sm text-muted-foreground mt-1">
                {selectedShows.length} shows, {selectedArtists.length} artists,{' '}
                {selectedVenues.length} venues selected
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="live-import"
                checked={!isDryRun}
                onCheckedChange={(checked) => setIsDryRun(!checked)}
              />
              <Label htmlFor="live-import" className="cursor-pointer">
                Live Import
              </Label>
            </div>
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          <div className="flex items-center gap-4">
            <Button
              onClick={handleImport}
              disabled={isImporting || !hasTargetToken}
              variant={isDryRun ? 'default' : 'destructive'}
            >
              {isImporting && <LoadingSpinner size="sm" />}
              {isDryRun ? 'Preview Import' : 'Import Now'}
            </Button>
            <Badge variant={isProduction ? 'destructive' : 'secondary'}>
              Target: {targetEnv}
            </Badge>
          </div>
        </CardContent>
      </Card>

      {error && (
        <ErrorAlert
          message={error instanceof Error ? error.message : 'Failed to import data'}
        />
      )}

      {importResult && (
        <ImportResults result={importResult} isDryRun={isDryRun} />
      )}
    </div>
  )
}

function ImportResults({
  result,
  isDryRun,
}: {
  result: DataImportResult
  isDryRun: boolean
}) {
  const prefix = isDryRun ? 'Would import' : 'Imported'

  return (
    <Card>
      <CardHeader>
        <CardTitle>{isDryRun ? 'Preview Results' : 'Import Results'}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {result.shows.total > 0 && (
          <ResultRow
            label="Shows"
            imported={result.shows.imported}
            duplicates={result.shows.duplicates}
            errors={result.shows.errors}
            prefix={prefix}
          />
        )}

        {result.artists.total > 0 && (
          <ResultRow
            label="Artists"
            imported={result.artists.imported}
            duplicates={result.artists.duplicates}
            errors={result.artists.errors}
            prefix={prefix}
          />
        )}

        {result.venues.total > 0 && (
          <ResultRow
            label="Venues"
            imported={result.venues.imported}
            duplicates={result.venues.duplicates}
            errors={result.venues.errors}
            prefix={prefix}
          />
        )}

        {/* Messages */}
        {(result.shows.messages.length > 0 ||
          result.artists.messages.length > 0 ||
          result.venues.messages.length > 0) && (
          <div>
            <h4 className="text-sm font-medium text-foreground mb-2">Details</h4>
            <div className="bg-muted rounded-lg p-3 max-h-48 overflow-y-auto">
              <ul className="text-xs font-mono space-y-1">
                {[
                  ...result.shows.messages,
                  ...result.artists.messages,
                  ...result.venues.messages,
                ].map((msg, i) => (
                  <li
                    key={i}
                    className={cn(
                      msg.startsWith('IMPORTED') || msg.startsWith('WOULD IMPORT')
                        ? 'text-green-600'
                        : msg.startsWith('DUPLICATE')
                        ? 'text-blue-600'
                        : msg.startsWith('ERROR') || msg.startsWith('SKIP')
                        ? 'text-destructive'
                        : 'text-muted-foreground'
                    )}
                  >
                    {msg}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ResultRow({
  label,
  imported,
  duplicates,
  errors,
  prefix,
}: {
  label: string
  imported: number
  duplicates: number
  errors: number
  prefix: string
}) {
  return (
    <div>
      <h4 className="text-sm font-medium text-foreground mb-2">{label}</h4>
      <div className="flex gap-4 text-sm">
        <span className="text-green-600">
          {prefix}: {imported}
        </span>
        <span className="text-blue-600">Duplicates: {duplicates}</span>
        <span className="text-destructive">Errors: {errors}</span>
      </div>
    </div>
  )
}
