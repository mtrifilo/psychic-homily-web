'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Flag,
  CheckCircle,
  XCircle,
  AlertCircle,
  UserX,
  ExternalLink,
  Music,
} from 'lucide-react'
import type { ArtistReportResponse } from '@/lib/types/artist'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { formatAdminDate } from '@/lib/utils/formatters'
import { DismissArtistReportDialog } from './DismissArtistReportDialog'
import { ResolveArtistReportDialog } from './ResolveArtistReportDialog'

interface ArtistReportCardProps {
  report: ArtistReportResponse
}

/**
 * Get the report type display info
 */
function getReportTypeInfo(type: string): {
  label: string
  icon: React.ReactNode
  color: string
} {
  switch (type) {
    case 'inaccurate':
      return {
        label: 'Inaccurate',
        icon: <AlertCircle className="h-4 w-4" />,
        color: 'text-amber-500 border-amber-500/50',
      }
    case 'removal_request':
      return {
        label: 'Removal Request',
        icon: <UserX className="h-4 w-4" />,
        color: 'text-red-500 border-red-500/50',
      }
    default:
      return {
        label: type,
        icon: <Flag className="h-4 w-4" />,
        color: 'text-muted-foreground border-border',
      }
  }
}

export function ArtistReportCard({ report }: ArtistReportCardProps) {
  const [showDismissDialog, setShowDismissDialog] = useState(false)
  const [showResolveDialog, setShowResolveDialog] = useState(false)

  const typeInfo = getReportTypeInfo(report.report_type)
  const artistName = report.artist?.name || 'Unknown Artist'
  const artistSlug = report.artist?.slug

  return (
    <>
      <Card className="border-orange-500/30 bg-card/50">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="font-semibold text-lg truncate">
                  {artistName}
                </h3>
                {artistSlug && (
                  <Link
                    href={`/artists/${artistSlug}`}
                    target="_blank"
                    className="text-muted-foreground hover:text-foreground"
                    title="View artist"
                  >
                    <ExternalLink className="h-4 w-4" />
                  </Link>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2 mt-1">
                <Badge
                  variant="outline"
                  className="text-purple-500 border-purple-500/50"
                >
                  <Music className="h-3 w-3 mr-1" />
                  Artist
                </Badge>
                <Badge variant="outline" className={typeInfo.color}>
                  <span className="mr-1">{typeInfo.icon}</span>
                  {typeInfo.label}
                </Badge>
              </div>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-4">
          <div className="grid gap-3 text-sm">
            {/* Report Details */}
            {report.details && (
              <div className="p-3 rounded-md bg-muted/50">
                <div className="text-xs font-medium text-muted-foreground mb-1">
                  Reporter&apos;s Details:
                </div>
                <p className="text-sm">{report.details}</p>
              </div>
            )}

            {/* Report Metadata */}
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Flag className="h-3 w-3" />
              <span>Reported on {formatAdminDate(report.created_at)}</span>
            </div>
          </div>

          {/* Actions */}
          <div className="flex gap-2 pt-2 border-t border-border/50">
            <Button
              variant="default"
              size="sm"
              className="flex-1 gap-2"
              onClick={() => setShowResolveDialog(true)}
            >
              <CheckCircle className="h-4 w-4" />
              Resolve
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="flex-1 gap-2 text-muted-foreground hover:text-muted-foreground"
              onClick={() => setShowDismissDialog(true)}
            >
              <XCircle className="h-4 w-4" />
              Dismiss
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Dialogs */}
      <DismissArtistReportDialog
        report={report}
        open={showDismissDialog}
        onOpenChange={setShowDismissDialog}
      />
      <ResolveArtistReportDialog
        report={report}
        open={showResolveDialog}
        onOpenChange={setShowResolveDialog}
      />
    </>
  )
}
