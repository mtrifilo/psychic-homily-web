import { useState } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
import { Checkbox } from './ui/checkbox'
import { Label } from './ui/label'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { LoadingSpinner } from './shared/LoadingSpinner'
import { ErrorAlert } from './shared/ErrorAlert'
import { ConfirmDialog } from './shared/ConfirmDialog'
import { AlertTriangle } from 'lucide-react'
import { cn } from '../lib/utils'
import { importEvents } from '../lib/api'
import type { ScrapedEvent, ImportResult, AppSettings } from '../lib/types'

interface Props {
  events: ScrapedEvent[]
  settings: AppSettings
  onBack: () => void
  onStartOver: () => void
}

export function ImportPanel({ events, settings, onBack, onStartOver }: Props) {
  const [loading, setLoading] = useState(false)
  const [isDryRun, setIsDryRun] = useState(true)
  const [result, setResult] = useState<ImportResult | null>(null)
  const [error, setError] = useState<string>('')
  const [showStartOverDialog, setShowStartOverDialog] = useState(false)

  const handleImport = async () => {
    setLoading(true)
    setError('')
    setResult(null)

    try {
      const importResult = await importEvents(events, isDryRun)
      setResult(importResult)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setLoading(false)
    }
  }

  const targetEnv = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'
  const isProduction = settings.targetEnvironment === 'production'
  const hasToken = isProduction
    ? Boolean(settings.productionToken?.length)
    : Boolean(settings.stageToken?.length)

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-foreground">Import Events</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Review and import {events.length} scraped event{events.length !== 1 ? 's' : ''} to {targetEnv}
        </p>
      </div>

      {!hasToken && (
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>{targetEnv} API Token Required</AlertTitle>
          <AlertDescription>
            Go to Settings to configure your {targetEnv} API token before importing.
          </AlertDescription>
        </Alert>
      )}

      {error && <ErrorAlert message={error} />}

      {/* Event Summary */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Events to Import</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="max-h-64 overflow-y-auto">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 sticky top-0">
                <tr>
                  <th className="px-4 py-2 text-left text-muted-foreground font-medium">Date</th>
                  <th className="px-4 py-2 text-left text-muted-foreground font-medium">Event</th>
                  <th className="px-4 py-2 text-left text-muted-foreground font-medium">Venue</th>
                  <th className="px-4 py-2 text-left text-muted-foreground font-medium">Artists</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {events.map(event => (
                  <tr key={`${event.venueSlug}-${event.id}`} className="hover:bg-muted/30">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap">
                      {formatDate(event.date)}
                    </td>
                    <td className="px-4 py-2 text-foreground">{event.title}</td>
                    <td className="px-4 py-2 text-muted-foreground">{event.venue}</td>
                    <td className="px-4 py-2 text-muted-foreground truncate max-w-xs">
                      {event.artists.join(', ')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Import Options */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-medium text-foreground">Import Mode</h3>
              <p className="text-sm text-muted-foreground mt-1">
                {isDryRun
                  ? 'Preview what would be imported without making changes'
                  : 'Import events to the database'}
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

          <div className="mt-4 flex items-center justify-between">
            <Badge variant={isProduction ? 'destructive' : 'secondary'}>
              Target: {targetEnv}
            </Badge>
            <Button
              onClick={handleImport}
              disabled={loading || !hasToken}
              variant={isDryRun ? 'default' : 'destructive'}
            >
              {loading && <LoadingSpinner size="sm" />}
              {isDryRun ? 'Preview Import' : 'Import Now'}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Import Results */}
      {result && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {isDryRun ? 'Preview Results' : 'Import Results'}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <StatCard label="Total" value={result.total} color="gray" />
              <StatCard
                label={isDryRun ? 'Would Import' : 'Imported'}
                value={result.imported}
                color="green"
              />
              <StatCard label="Duplicates" value={result.duplicates} color="blue" />
              <StatCard label="Rejected" value={result.rejected} color="amber" />
              {result.pending_review > 0 && (
                <StatCard
                  label={isDryRun ? 'Would Flag' : 'Flagged'}
                  value={result.pending_review}
                  color="amber"
                />
              )}
              {result.errors > 0 && (
                <StatCard label="Errors" value={result.errors} color="red" />
              )}
            </div>

            {result.messages.length > 0 && (
              <div>
                <h4 className="text-sm font-medium text-foreground mb-2">Details</h4>
                <div className="bg-muted rounded-lg p-3 max-h-48 overflow-y-auto">
                  <ul className="text-xs font-mono space-y-1">
                    {result.messages.map((msg, i) => (
                      <li
                        key={i}
                        className={cn(
                          msg.startsWith('IMPORTED') || msg.startsWith('WOULD IMPORT')
                            ? 'text-green-600'
                            : msg.startsWith('DUPLICATE')
                            ? 'text-blue-600'
                            : msg.startsWith('FLAGGED FOR REVIEW') || msg.startsWith('WOULD FLAG FOR REVIEW')
                            ? 'text-amber-600'
                            : msg.startsWith('REJECTED')
                            ? 'text-amber-600'
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
      )}

      <div className="flex justify-between">
        <Button variant="ghost" onClick={onBack}>
          Back
        </Button>
        <Button variant="outline" onClick={() => setShowStartOverDialog(true)}>
          Start Over
        </Button>
      </div>

      <ConfirmDialog
        open={showStartOverDialog}
        onOpenChange={setShowStartOverDialog}
        title="Start Over?"
        description="This will clear all your selections and scraped events. You'll need to start the process from the beginning."
        confirmLabel="Start Over"
        variant="destructive"
        onConfirm={onStartOver}
      />
    </div>
  )
}

function StatCard({
  label,
  value,
  color,
}: {
  label: string
  value: number
  color: 'gray' | 'green' | 'blue' | 'amber' | 'red'
}) {
  const colorClasses = {
    gray: 'bg-muted text-foreground',
    green: 'bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400',
    blue: 'bg-blue-100 text-blue-700 dark:bg-blue-950/50 dark:text-blue-400',
    amber: 'bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400',
    red: 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-400',
  }

  return (
    <div className={cn('rounded-lg p-3', colorClasses[color])}>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-sm">{label}</div>
    </div>
  )
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  })
}
