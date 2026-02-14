'use client'

import { useState } from 'react'
import { usePathname } from 'next/navigation'
import { Flag, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useMyArtistReport } from '@/lib/hooks/useArtistReports'
import { useAuthContext } from '@/lib/context/AuthContext'
import { ReportArtistDialog } from './ReportArtistDialog'
import { LoginPromptDialog } from '@/components/auth/LoginPromptDialog'

interface ReportArtistButtonProps {
  artistId: number
  artistName: string
  variant?: 'default' | 'ghost' | 'outline'
  size?: 'sm' | 'default' | 'lg'
}

export function ReportArtistButton({
  artistId,
  artistName,
  variant = 'outline',
  size = 'sm',
}: ReportArtistButtonProps) {
  const pathname = usePathname()
  const { isAuthenticated } = useAuthContext()
  const { data: myReport, isLoading } = useMyArtistReport(
    isAuthenticated ? artistId : null
  )
  const [isReportDialogOpen, setIsReportDialogOpen] = useState(false)
  const [isLoginPromptOpen, setIsLoginPromptOpen] = useState(false)

  const hasReported = myReport?.report !== null

  // If user has already reported, show a disabled "Reported" button
  if (isAuthenticated && hasReported) {
    return (
      <Button
        variant="outline"
        size={size}
        disabled
        className="text-muted-foreground"
        title="You have already reported this artist"
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
        title="Report an issue with this artist"
      >
        <Flag className="h-4 w-4 mr-2" />
        Report Issue
      </Button>

      {isAuthenticated && (
        <ReportArtistDialog
          artistId={artistId}
          artistName={artistName}
          open={isReportDialogOpen}
          onOpenChange={setIsReportDialogOpen}
        />
      )}

      {!isAuthenticated && (
        <LoginPromptDialog
          open={isLoginPromptOpen}
          onOpenChange={setIsLoginPromptOpen}
          title="Sign in to report"
          description="You need to be signed in to report an issue with this artist. This helps us prevent abuse and keep our community safe."
          returnTo={pathname}
        />
      )}
    </>
  )
}
