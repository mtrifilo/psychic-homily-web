'use client'

import { useState } from 'react'
import { usePathname } from 'next/navigation'
import { Flag, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useMyShowReport } from '../hooks/useShowReports'
import { useAuthContext } from '@/lib/context/AuthContext'
import { ReportShowDialog } from './ReportShowDialog'
import { LoginPromptDialog } from '@/features/auth'

interface ReportShowButtonProps {
  showId: number
  showTitle: string
  variant?: 'default' | 'ghost' | 'outline'
  size?: 'sm' | 'default' | 'lg'
}

export function ReportShowButton({
  showId,
  showTitle,
  variant = 'outline',
  size = 'sm',
}: ReportShowButtonProps) {
  const pathname = usePathname()
  const { isAuthenticated } = useAuthContext()
  const { data: myReport, isLoading } = useMyShowReport(
    isAuthenticated ? showId : null
  )
  const [isReportDialogOpen, setIsReportDialogOpen] = useState(false)
  const [isLoginPromptOpen, setIsLoginPromptOpen] = useState(false)

  // PSY-476: `myReport?.report !== null` is true when the query is still
  // loading (`myReport` undefined → `undefined !== null` → true), which
  // flashed the disabled "Reported" state before real data arrived. Gate
  // on `!isLoading` and use loose `!= null` so both `undefined` and `null`
  // mean "no existing report".
  const hasReported = !isLoading && myReport?.report != null

  // If user has already reported, show a disabled "Reported" button
  if (isAuthenticated && hasReported) {
    return (
      <Button
        variant="outline"
        size={size}
        disabled
        className="text-muted-foreground"
        title="You have already reported this show"
      >
        <Check className="h-4 w-4 mr-2" />
        Reported
      </Button>
    )
  }

  const handleClick = () => {
    if (isAuthenticated) {
      setIsReportDialogOpen(true)
    } else {
      setIsLoginPromptOpen(true)
    }
  }

  return (
    <>
      <Button
        variant={variant}
        size={size}
        onClick={handleClick}
        disabled={isLoading}
        title="Report an issue with this show"
      >
        <Flag className="h-4 w-4 mr-2" />
        Report Issue
      </Button>

      {isAuthenticated && (
        <ReportShowDialog
          showId={showId}
          showTitle={showTitle}
          open={isReportDialogOpen}
          onOpenChange={setIsReportDialogOpen}
        />
      )}

      {!isAuthenticated && (
        <LoginPromptDialog
          open={isLoginPromptOpen}
          onOpenChange={setIsLoginPromptOpen}
          title="Sign in to report"
          description="You need to be signed in to report an issue with this show. This helps us prevent abuse and keep our community safe."
          returnTo={pathname}
        />
      )}
    </>
  )
}
