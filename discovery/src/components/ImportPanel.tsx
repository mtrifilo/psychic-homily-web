import { useState, useEffect } from 'react'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
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
  const [result, setResult] = useState<ImportResult | null>(null)
  const [isLiveResult, setIsLiveResult] = useState(false)
  const [error, setError] = useState<string>('')
  const [showStartOverDialog, setShowStartOverDialog] = useState(false)
  const [previewDone, setPreviewDone] = useState(false)

  const targetEnv = settings.targetEnvironment === 'production' ? 'Production' : 'Stage'
  const isProduction = settings.targetEnvironment === 'production'
  const hasToken = isProduction
    ? Boolean(settings.productionToken?.length)
    : Boolean(settings.stageToken?.length)

  // Auto-run preview (dry run) on mount
  useEffect(() => {
    if (!hasToken || previewDone) return
    let cancelled = false

    const runPreview = async () => {
      setLoading(true)
      setError('')
      try {
        const importResult = await importEvents(events, true)
        if (!cancelled) {
          setResult(importResult)
          setPreviewDone(true)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Preview failed')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    runPreview()
    return () => { cancelled = true }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const handleLiveImport = async () => {
    setLoading(true)
    setError('')
    setResult(null)
    setIsLiveResult(true)

    try {
      const importResult = await importEvents(events, false)
      setResult(importResult)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed')
    } finally {
      setLoading(false)
    }
  }

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
                  <th className="px-4 py-2 text-left text-muted-foreground font-medium">Info</th>
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
                    <td className="px-4 py-2">
                      <div className="flex gap-1 flex-wrap">
                        {event.price && (
                          <Badge variant="outline" className="text-xs">{event.price}</Badge>
                        )}
                        {event.ageRestriction && (
                          <Badge variant="outline" className="text-xs">{event.ageRestriction}</Badge>
                        )}
                        {event.isSoldOut && (
                          <Badge className="bg-red-100 text-red-800 hover:bg-red-100 text-xs">Sold Out</Badge>
                        )}
                        {event.isCancelled && (
                          <Badge className="bg-gray-100 text-gray-800 hover:bg-gray-100 text-xs">Cancelled</Badge>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Import Action */}
      <Card>
        <CardContent className="pt-6 flex items-center justify-between">
          <Badge variant={isProduction ? 'destructive' : 'secondary'}>
            Target: {targetEnv}
          </Badge>
          <Button
            onClick={handleLiveImport}
            disabled={loading || !hasToken}
            variant="destructive"
          >
            {loading && <LoadingSpinner size="sm" />}
            Import Now
          </Button>
        </CardContent>
      </Card>

      {/* Results */}
      {loading && !result && (
        <Card>
          <CardContent className="pt-6 flex items-center justify-center gap-2 text-muted-foreground">
            <LoadingSpinner size="sm" />
            <span>Running preview...</span>
          </CardContent>
        </Card>
      )}

      {result && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {isLiveResult ? 'Import Results' : 'Preview Results'}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <StatCard label="Total" value={result.total} color="gray" />
              <StatCard
                label={isLiveResult ? 'Imported' : 'Would Import'}
                value={result.imported}
                color="green"
              />
              <StatCard label="Duplicates" value={result.duplicates} color="gray" />
              {result.updated > 0 && (
                <StatCard
                  label={isLiveResult ? 'Updated' : 'Would Update'}
                  value={result.updated}
                  color="blue"
                />
              )}
              <StatCard label="Rejected" value={result.rejected} color="amber" />
              {result.pending_review > 0 && (
                <StatCard
                  label={isLiveResult ? 'Flagged' : 'Would Flag'}
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
                <div className="bg-muted rounded-lg p-3 max-h-96 overflow-y-auto space-y-3">
                  {result.messages.map((msg, i) => {
                    const event = events[i]
                    const statusColor = msg.startsWith('IMPORTED') || msg.startsWith('WOULD IMPORT')
                      ? 'text-green-600'
                      : msg.startsWith('UPDATED') || msg.startsWith('WOULD UPDATE')
                      ? 'text-blue-600'
                      : msg.startsWith('DUPLICATE')
                      ? 'text-muted-foreground'
                      : msg.startsWith('FLAGGED FOR REVIEW') || msg.startsWith('WOULD FLAG FOR REVIEW')
                      ? 'text-amber-600'
                      : msg.startsWith('REJECTED')
                      ? 'text-amber-600'
                      : msg.startsWith('ERROR') || msg.startsWith('SKIP')
                      ? 'text-destructive'
                      : 'text-muted-foreground'

                    return (
                      <div key={i}>
                        <div className={cn('text-xs font-mono font-semibold', statusColor)}>
                          {msg}
                        </div>
                        {event && (
                          <ul className="text-xs text-muted-foreground mt-1 ml-4 space-y-0.5">
                            <li>Date: {new Date(event.date + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' })}</li>
                            <li>Artists: {event.artists.join(', ')}</li>
                            {event.showTime && <li>Show: {event.showTime}</li>}
                            {event.doorsTime && <li>Doors: {event.doorsTime}</li>}
                            {event.price && <li>Price: {event.price}</li>}
                            {event.ageRestriction && <li>Ages: {event.ageRestriction}</li>}
                            {event.ticketUrl && (
                              <li>
                                Tickets:{' '}
                                <a
                                  href={event.ticketUrl}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="text-blue-500 hover:underline"
                                >
                                  {event.ticketUrl.length > 60
                                    ? event.ticketUrl.slice(0, 60) + '...'
                                    : event.ticketUrl}
                                </a>
                              </li>
                            )}
                            {event.isSoldOut && <li className="text-red-500">Sold Out</li>}
                            {event.isCancelled && <li className="text-red-500">Cancelled</li>}
                          </ul>
                        )}
                      </div>
                    )
                  })}
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
