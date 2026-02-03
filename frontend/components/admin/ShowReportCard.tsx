'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Calendar,
  Flag,
  CheckCircle,
  XCircle,
  CalendarX,
  BanIcon,
  AlertCircle,
  ExternalLink,
} from 'lucide-react'
import type { ShowReportResponse } from '@/lib/types/show'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DismissReportDialog } from './DismissReportDialog'
import { ResolveReportDialog } from './ResolveReportDialog'

interface ShowReportCardProps {
  report: ShowReportResponse
}

/**
 * Format date for display
 */
function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
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
    case 'cancelled':
      return {
        label: 'Cancelled',
        icon: <CalendarX className="h-4 w-4" />,
        color: 'text-red-500 border-red-500/50',
      }
    case 'sold_out':
      return {
        label: 'Sold Out',
        icon: <BanIcon className="h-4 w-4" />,
        color: 'text-orange-500 border-orange-500/50',
      }
    case 'inaccurate':
      return {
        label: 'Inaccurate',
        icon: <AlertCircle className="h-4 w-4" />,
        color: 'text-amber-500 border-amber-500/50',
      }
    default:
      return {
        label: type,
        icon: <Flag className="h-4 w-4" />,
        color: 'text-muted-foreground border-border',
      }
  }
}

export function ShowReportCard({ report }: ShowReportCardProps) {
  const [showDismissDialog, setShowDismissDialog] = useState(false)
  const [showResolveDialog, setShowResolveDialog] = useState(false)

  const typeInfo = getReportTypeInfo(report.report_type)
  const showTitle = report.show?.title || 'Unknown Show'
  const showSlug = report.show?.slug

  return (
    <>
      <Card className="border-orange-500/30 bg-card/50">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="font-semibold text-lg truncate">{showTitle}</h3>
                {showSlug && (
                  <Link
                    href={`/shows/${showSlug}`}
                    target="_blank"
                    className="text-muted-foreground hover:text-foreground"
                    title="View show"
                  >
                    <ExternalLink className="h-4 w-4" />
                  </Link>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2 mt-1">
                <Badge variant="outline" className={typeInfo.color}>
                  <span className="mr-1">{typeInfo.icon}</span>
                  {typeInfo.label}
                </Badge>
              </div>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-4">
          {/* Event Details */}
          <div className="grid gap-3 text-sm">
            {/* Event Date */}
            {report.show?.event_date && (
              <div className="flex items-center gap-2 text-muted-foreground">
                <Calendar className="h-4 w-4 shrink-0" />
                <span>{formatDate(report.show.event_date)}</span>
                {report.show.city && report.show.state && (
                  <span className="text-xs">
                    ({report.show.city}, {report.show.state})
                  </span>
                )}
              </div>
            )}

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
              <span>Reported on {formatDate(report.created_at)}</span>
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
      <DismissReportDialog
        report={report}
        open={showDismissDialog}
        onOpenChange={setShowDismissDialog}
      />
      <ResolveReportDialog
        report={report}
        open={showResolveDialog}
        onOpenChange={setShowResolveDialog}
      />
    </>
  )
}
